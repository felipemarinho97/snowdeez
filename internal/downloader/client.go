package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/felipemarinho97/godeez/internal/bpm"
	"github.com/felipemarinho97/godeez/internal/config"
	"github.com/felipemarinho97/godeez/internal/crypto"
	"github.com/felipemarinho97/godeez/internal/deezer"
	"github.com/felipemarinho97/godeez/internal/fileutil"
	"github.com/felipemarinho97/godeez/internal/logger"
	"github.com/felipemarinho97/godeez/internal/store"
	"github.com/felipemarinho97/godeez/internal/tags"
	"github.com/flytam/filenamify"
)

const chunkSize = 2048

type Client struct {
	appConfig    *config.Config
	resourceType string
	deezerClient *deezer.Client
	Logger       *logger.Logger

	hashIndexOnce sync.Once
	hashIndex     *fileutil.HashIndex
	hashIndexErr  error
}

func New(appConfig *config.Config, resourceType string) *Client {
	return &Client{
		appConfig:    appConfig,
		resourceType: resourceType,
		deezerClient: nil,
		Logger:       logger.New(nil), // Initialize with a nil logger, can be set later
	}
}

func (c *Client) Run(ctx context.Context, opts Options, id string) error {
	var err error
	c.deezerClient, err = deezer.NewClient(ctx, c.appConfig)
	if err != nil {
		return err
	}

	if !c.deezerClient.Session.Premium && (opts.Quality == "mp3_320" || opts.Quality == "flac") {
		return fmt.Errorf("premium account required for '%s' quality", opts.Quality)
	}

	var resource deezer.Resource
	switch c.resourceType {
	case "album":
		resource = &deezer.Album{}
	case "playlist":
		resource = &deezer.Playlist{}
	case "artist":
		resource = &deezer.Artist{}
	case "track":
		resource = &deezer.Track{}
	case "show":
		resource = &deezer.Show{}
	case "episode":
		resource = &deezer.Talk{}
	default:
		return fmt.Errorf("unsupported resource type: %s", c.resourceType)
	}

	if err := c.deezerClient.FetchResource(ctx, resource, id); err != nil {
		return fmt.Errorf("failed to fetch resource: %w", err)
	}

	songs := resource.GetSongs()
	if len(songs) == 0 {
		switch c.resourceType {
		case "show":
			return fmt.Errorf("show has no episodes")
		default:
			return fmt.Errorf("%s has no songs", c.resourceType)
		}
	}
	if (c.resourceType == "artist" || c.resourceType == "show") && len(songs) > opts.Limit {
		songs = songs[:opts.Limit]
		resource.SetSongs(songs)
	}

	rootOutputDir := c.appConfig.OutputDir
	resourceOutputDir := resource.GetOutputDir(rootOutputDir)
	if err := fileutil.EnsureDir(resourceOutputDir); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	startTime := time.Now()
	fmt.Printf("%s\n\nStarting download...\n\n", resource)

	downloaded := 0
	skipped := 0
	failed := 0

	for i, song := range songs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		trackProgress := fmt.Sprintf("[%d/%d]", i+1, len(songs))

		sp := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		sp.Writer = os.Stdout
		sp.Prefix = trackProgress + " "
		sp.Suffix = fmt.Sprintf(" Downloading: %s - %s", song.Artist, song.Title)
		sp.Start()

		warnings, err := c.downloadSong(ctx, resource, song, opts, resourceOutputDir)
		sp.Stop()

		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}

			if path, ok := IsSkipError(err); ok {
				skipped++
				fmt.Printf("%s ↷ Skipped: %s - %s\n    Already exists at: %s\n", trackProgress, song.Artist, song.Title, path)
				continue
			}

			failed++
			c.Logger.Errorf("Failed to download %s - %s: %v\n", song.Artist, song.Title, err)
			fmt.Printf("%s ✖ Failed: %s - %s:\n    Error: %v\n", trackProgress, song.Artist, song.Title, err)

			continue
		}

		symbol := "✔"
		if len(warnings) > 0 {
			symbol = "⚠"
		}

		downloaded++
		c.Logger.Infof("Downloaded %s - %s\n", song.Artist, song.Title)
		fmt.Printf("%s %s Downloaded: %s - %s\n", trackProgress, symbol, song.Artist, song.Title)

		for _, w := range warnings {
			c.Logger.Warnf("Warning: %s\n", w)
			fmt.Printf("    Warning: %s\n", w)
		}
	}

	if downloaded > 0 || failed > 0 {
		c.Logger.Infof("Playlist %s (%s): %d downloaded, %d skipped, %d failed\n", resource.GetTitle(), id, downloaded, skipped, failed)
	}
	fmt.Printf(`
================== [ Summary ] ==================
Downloaded:     %d
Skipped:        %d
Failed:         %d
Elapsed time:   %s
Files saved to: %s
=================================================
`,
		downloaded,
		skipped,
		failed,
		time.Since(startTime).Round(time.Second),
		resourceOutputDir,
	)

	// Create M3U playlist file for playlists
	if c.resourceType == "playlist" && downloaded > 0 {
		if err := c.createM3UPlaylist(resource, resourceOutputDir); err != nil {
			c.Logger.Warnf("Failed to create M3U playlist: %v\n", err)
			fmt.Printf("Warning: Failed to create M3U playlist: %v\n", err)
		} else {
			fmt.Printf("Playlist file created: %s.m3u\n", resource.GetTitle())
		}
	}

	return nil
}

func (c *Client) downloadSong(ctx context.Context, resource deezer.Resource, song *deezer.Song, opts Options, outputDir string) ([]string, error) {
	var warnings []string

	if c.resourceType == "show" || c.resourceType == "episode" {
		return c.downloadEpisode(ctx, resource, song, opts, outputDir)
	}

	media, err := c.deezerClient.FetchMedia(ctx, song, opts.Quality)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch media: %w", err)
	}

	// Use the organized tree structure for all songs
	var outputPath string
	if c.resourceType == "show" {
		// For shows, we need to use the episode's organized path method
		// First, find the original episode data from the resource
		if show, ok := resource.(*deezer.Show); ok {
			// Find the matching episode
			for _, episode := range show.Results.Episodes.Data {
				if episode.EpisodeID == song.ID {
					outputPath = episode.GetOrganizedPath(c.appConfig.OutputDir, media)
					break
				}
			}
		}
		if outputPath == "" {
			outputPath = song.GetOrganizedPath(c.appConfig.OutputDir, media)
		}
	} else {
		outputPath = song.GetOrganizedPath(c.appConfig.OutputDir, media)
	}

	// Ensure the directory exists
	if err := fileutil.EnsureDir(outputPath[:len(outputPath)-len(path.Base(outputPath))]); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	mediaFormat, err := media.GetFormat()
	if err != nil {
		return nil, fmt.Errorf("failed to get media format: %w", err)
	}
	if opts.Strict && strings.ToLower(mediaFormat) != opts.Quality {
		return nil, fmt.Errorf("requested quality '%s' not available", opts.Quality)
	}

	if path, skip := c.shouldSkipDownload(ctx, song.ID, mediaFormat); skip {
		return nil, SkipError{Path: path}
	}

	var metricsChan chan *bpm.Metrics
	var errChan chan error
	if opts.BPM {
		metricsChan = make(chan *bpm.Metrics, 1)
		errChan = make(chan error, 1)
		go func() {
			metrics, err := bpm.FetchMetrics(ctx, c.deezerClient.Session.HttpClient, song.Artist, song.Title, song.Duration)
			if err != nil {
				errChan <- err
				return
			}

			metricsChan <- metrics
		}()
	}

	stream, err := c.deezerClient.GetMediaStream(ctx, media, song.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get media stream: %w", err)
	}

	dlCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	key := crypto.GetKey(c.appConfig.SecretKey, song.ID)
	if err := c.streamToFile(dlCtx, stream, outputPath, key); err != nil {
		fileutil.DeleteFile(outputPath)

		return nil, fmt.Errorf("failed to stream to file: %w", err)
	}

	if opts.Quality != strings.ToLower(mediaFormat) {
		warnings = append(warnings, fmt.Sprintf("requested quality '%s' not available, using '%s' instead", opts.Quality, strings.ToLower(mediaFormat)))
	}

	metrics := &bpm.Metrics{}
	if opts.BPM {
		select {
		case metrics = <-metricsChan:
		case err := <-errChan:
			if !errors.Is(err, context.Canceled) {
				warnings = append(warnings, fmt.Sprintf("failed to fetch BPM and key: %v", err))
			}
		}
	}

	cover, err := c.deezerClient.FetchCoverImage(ctx, song)
	if err != nil && !errors.Is(err, context.Canceled) {
		warnings = append(warnings, fmt.Sprintf("failed to fetch cover image: %v", err))
	}

	warnings = append(warnings, c.finalizeDownload(resource, song, outputPath, mediaFormat, cover, metrics)...)

	return warnings, nil
}

func (c *Client) streamToFile(ctx context.Context, stream io.ReadCloser, outputPath string, key []byte) error {
	defer stream.Close()

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	buffer := make([]byte, chunkSize)
	for chunk := 0; ; chunk++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// continue
		}

		totalRead := 0
		for totalRead < chunkSize {
			n, err := stream.Read(buffer[totalRead:])
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return err
			}

			if n > 0 {
				totalRead += n
			}
		}

		if totalRead == 0 {
			break
		}

		if chunk%3 == 0 && totalRead == chunkSize {
			buffer, err = crypto.Decrypt(buffer, key)
			if err != nil {
				return err
			}
		}

		_, err = file.Write(buffer[:totalRead])
		if err != nil {
			return err
		}

		if totalRead < chunkSize {
			break
		}
	}

	return nil
}

// streamToFileNoCrypto streams data directly to file without decryption (for episodes)
func (c *Client) streamToFileNoCrypto(ctx context.Context, stream io.ReadCloser, outputPath string) error {
	defer stream.Close()

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	return err
}

func (c *Client) finalizeDownload(resource deezer.Resource, song *deezer.Song, outputPath, mediaFormat string, cover []byte, metrics *bpm.Metrics) []string {
	var warnings []string

	if err := tags.AddTags(resource, song, cover, outputPath, metrics.BPM, metrics.Key); err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to add tags: %v", err))
	}

	hash, err := fileutil.GetFileHash(outputPath)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to get file hash: %v", err))
	}

	info := &store.DownloadInfo{
		SongID:     song.ID,
		Quality:    mediaFormat,
		Path:       outputPath,
		Hash:       hash,
		Downloaded: time.Now(),
	}

	if err := info.Save(); err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to save download info: %v", err))
	}

	return warnings
}

func (c *Client) initHashIndex(ctx context.Context) error {
	c.hashIndexOnce.Do(func() {
		c.hashIndex, c.hashIndexErr = fileutil.NewHashIndex(ctx, c.appConfig.OutputDir)
	})

	return c.hashIndexErr
}

// createM3UPlaylist creates an M3U playlist file with relative paths to the distributed song files
func (c *Client) createM3UPlaylist(resource deezer.Resource, playlistDir string) error {
	songs := resource.GetSongs()
	if len(songs) == 0 {
		return fmt.Errorf("no songs to add to playlist")
	}

	playlistPath := path.Join(playlistDir, resource.GetTitle()+".m3u")
	file, err := os.Create(playlistPath)
	if err != nil {
		return fmt.Errorf("failed to create playlist file: %w", err)
	}
	defer file.Close()

	// Write M3U header
	if _, err := file.WriteString("#EXTM3U\n"); err != nil {
		return fmt.Errorf("failed to write M3U header: %w", err)
	}

	for _, song := range songs {
		// Create a mock media object to determine file extension
		// We'll assume mp3 for the M3U, but this should ideally check the actual downloaded format
		mockMedia := &deezer.Media{
			Data: []struct {
				Media []struct {
					Type    string          `json:"media_type"`
					Cipher  deezer.Cipher   `json:"cipher"`
					Format  string          `json:"format"`
					Sources []deezer.Source `json:"sources"`
				}
				Errors []deezer.MediaError `json:"errors"`
			}{
				{
					Media: []struct {
						Type    string          `json:"media_type"`
						Cipher  deezer.Cipher   `json:"cipher"`
						Format  string          `json:"format"`
						Sources []deezer.Source `json:"sources"`
					}{
						{Format: "MP3_320"},
					},
				},
			},
		}

		// Get the organized path for this song
		songPath := song.GetOrganizedPath(c.appConfig.OutputDir, mockMedia)

		// Calculate relative path from playlist directory to song file
		relativePath, err := filepath.Rel(playlistDir, songPath)
		if err != nil {
			// If relative path calculation fails, use absolute path
			relativePath = songPath
		}

		// Write track info
		duration := "0"
		if song.Duration != "" {
			duration = song.Duration
		}

		trackInfo := fmt.Sprintf("#EXTINF:%s,%s - %s\n", duration, song.Artist, song.GetTitle())
		if _, err := file.WriteString(trackInfo); err != nil {
			return fmt.Errorf("failed to write track info: %w", err)
		}

		// Write file path
		if _, err := file.WriteString(relativePath + "\n"); err != nil {
			return fmt.Errorf("failed to write file path: %w", err)
		}
	}

	return nil
}

// downloadEpisode handles downloading podcast episodes which use direct streaming
func (c *Client) downloadEpisode(ctx context.Context, resource deezer.Resource, song *deezer.Song, opts Options, outputDir string) ([]string, error) {
	var warnings []string

	if song.EpisodeDirectStreamURL == "" {
		return nil, fmt.Errorf("episode has no direct stream URL (episode: %s)", song.ID)
	}

	mockMedia := &deezer.Media{
		Data: []struct {
			Media []struct {
				Type    string          `json:"media_type"`
				Cipher  deezer.Cipher   `json:"cipher"`
				Format  string          `json:"format"`
				Sources []deezer.Source `json:"sources"`
			}
			Errors []deezer.MediaError `json:"errors"`
		}{
			{
				Media: []struct {
					Type    string          `json:"media_type"`
					Cipher  deezer.Cipher   `json:"cipher"`
					Format  string          `json:"format"`
					Sources []deezer.Source `json:"sources"`
				}{
					{Format: "MP3_320"}, // Episodes are typically MP3
				},
			},
		},
	}

	var outputPath string
	if show, ok := resource.(*deezer.Show); ok {
		for _, episode := range show.Results.Episodes.Data {
			if episode.EpisodeID == song.ID {
				outputPath = episode.GetOrganizedPath(c.appConfig.OutputDir, mockMedia)
				break
			}
		}
	} else if talk, ok := resource.(*deezer.Talk); ok {
		// For individual episodes (Talk), create the proper path: Podcasts/ShowName/EpisodeName.mp3
		showName, _ := filenamify.Filenamify(talk.Results.ShowName, filenamify.Options{MaxLength: 1000})
		episodeName, _ := filenamify.Filenamify(talk.Results.EpisodeTitle, filenamify.Options{MaxLength: 1000})
		outputPath = path.Join(c.appConfig.OutputDir, "Podcasts", showName, episodeName+".mp3")
	}
	if outputPath == "" {
		outputPath = song.GetOrganizedPath(c.appConfig.OutputDir, mockMedia)
	}

	if err := fileutil.EnsureDir(outputPath[:len(outputPath)-len(path.Base(outputPath))]); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	if path, skip := c.shouldSkipDownload(ctx, song.ID, "mp3_320"); skip {
		return nil, SkipError{Path: path}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", song.EpisodeDirectStreamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: opts.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download episode stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from episode stream: %d", resp.StatusCode)
	}

	if err := c.streamToFileNoCrypto(ctx, resp.Body, outputPath); err != nil {
		fileutil.DeleteFile(outputPath)
		return nil, fmt.Errorf("failed to stream episode to file: %w", err)
	}

	if opts.BPM {
		warnings = append(warnings, "BPM fetching not supported for podcast episodes")
	}

	warnings = append(warnings, c.finalizeEpisodeDownload(song, outputPath, "mp3_320", nil)...)

	return warnings, nil
}

// finalizeEpisodeDownload is similar to finalizeDownload but for episodes
func (c *Client) finalizeEpisodeDownload(song *deezer.Song, outputPath, mediaFormat string, cover []byte) []string {
	var warnings []string

	tempShow := &deezer.Show{}
	tempShow.Results.Data.ShowName = song.Artist

	if err := tags.AddTags(tempShow, song, cover, outputPath, "", ""); err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to write tags: %v", err))
	}

	downloadInfo := &store.DownloadInfo{
		SongID:  song.ID,
		Path:    outputPath,
		Quality: mediaFormat,
	}

	if err := downloadInfo.Save(); err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to save download info: %v", err))
	}

	return warnings
}
