package deezer

import (
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/flytam/filenamify"
)

type Show struct {
	Results struct {
		Data struct {
			Available      bool   `json:"AVAILABLE"`
			LabelName      string `json:"LABEL_NAME"`
			ShowArtMD5     string `json:"SHOW_ART_MD5"`
			ShowDescription string `json:"SHOW_DESCRIPTION"`
			ShowEpisodeDisplayCount string `json:"SHOW_EPISODE_DISPLAY_COUNT"`
			ShowID         string `json:"SHOW_ID"`
			ShowIsAdvertisingAllowed string `json:"SHOW_IS_ADVERTISING_ALLOWED"`
			ShowIsDirectStream string `json:"SHOW_IS_DIRECT_STREAM"`
			ShowIsExplicit string `json:"SHOW_IS_EXPLICIT"`
			ShowName       string `json:"SHOW_NAME"`
			ShowStatus     string `json:"SHOW_STATUS"`
			ShowType       string `json:"SHOW_TYPE"`
			Type           string `json:"__TYPE__"`
		} `json:"DATA"`
		Episodes struct {
			Count int       `json:"count"`
			Data  []*Episode `json:"data"`
		} `json:"EPISODES"`
		FavoriteStatus bool `json:"FAVORITE_STATUS"`
	} `json:"results"`
}

type Episode struct {
	Available                 bool   `json:"AVAILABLE"`
	Duration                  string `json:"DURATION"`
	EpisodeDescription        string `json:"EPISODE_DESCRIPTION"`
	EpisodeDirectStreamURL    string `json:"EPISODE_DIRECT_STREAM_URL"`
	EpisodeID                 string `json:"EPISODE_ID"`
	EpisodeImageMD5           string `json:"EPISODE_IMAGE_MD5"`
	EpisodePublishedTimestamp string `json:"EPISODE_PUBLISHED_TIMESTAMP"`
	EpisodePublishedTS        int64  `json:"EPISODE_PUBLISHED_TS"`
	EpisodeStatus             string `json:"EPISODE_STATUS"`
	EpisodeTitle              string `json:"EPISODE_TITLE"`
	EpisodeUpdateTimestamp    string `json:"EPISODE_UPDATE_TIMESTAMP"`
	FilesizeMP3_32            string `json:"FILESIZE_MP3_32"`
	FilesizeMP3_64            string `json:"FILESIZE_MP3_64"`
	MD5Origin                 string `json:"MD5_ORIGIN"`
	ShowArtMD5                string `json:"SHOW_ART_MD5"`
	ShowDescription           string `json:"SHOW_DESCRIPTION"`
	ShowID                    string `json:"SHOW_ID"`
	ShowIsAdvertisingAllowed  string `json:"SHOW_IS_ADVERTISING_ALLOWED"`
	ShowIsDirectStream        string `json:"SHOW_IS_DIRECT_STREAM"`
	ShowIsDownloadAllowed     string `json:"SHOW_IS_DOWNLOAD_ALLOWED"`
	ShowIsExplicit            string `json:"SHOW_IS_EXPLICIT"`
	ShowName                  string `json:"SHOW_NAME"`
	TrackToken                string `json:"TRACK_TOKEN"`
	TrackTokenExpire          int64  `json:"TRACK_TOKEN_EXPIRE"`
	Type                      string `json:"__TYPE__"`
	
	// Song-like fields to make it compatible with existing download logic
	ID          string `json:"-"` // Will be set to EpisodeID
	Title       string `json:"-"` // Will be set to EpisodeTitle
	Artist      string `json:"-"` // Will be set to ShowName
	AlbumTitle  string `json:"-"` // Will be set to ShowName
	Cover       string `json:"-"` // Will be derived from EpisodeImageMD5 or ShowArtMD5
}

func (s *Show) String() string {
	return fmt.Sprintf(
		`================= [ Show Info ] ==================
Title:       %s
Description: %s
Episodes:    %d
Label:       %s
===================================================`,
		s.Results.Data.ShowName,
		s.Results.Data.ShowDescription,
		s.Results.Episodes.Count,
		s.Results.Data.LabelName,
	)
}

func (s *Show) GetType() string {
	return "Show"
}

func (s *Show) GetTitle() string {
	return s.Results.Data.ShowName
}

func (s *Show) GetSongs() []*Song {
	// Convert episodes to songs for compatibility with existing download logic
	songs := make([]*Song, len(s.Results.Episodes.Data))
	for i, episode := range s.Results.Episodes.Data {
		songs[i] = episode.ToSong()
	}
	return songs
}

func (s *Show) SetSongs(songs []*Song) {
	// Convert songs back to episodes
	episodes := make([]*Episode, len(songs))
	for i, song := range songs {
		episodes[i] = &Episode{
			EpisodeID:    song.ID,
			EpisodeTitle: song.Title,
			Duration:     song.Duration,
			ShowName:     song.Artist,
			TrackToken:   song.TrackToken,
			Available:    true,
			Type:         "episode",
		}
	}
	s.Results.Episodes.Data = episodes
}

func (s *Show) GetOutputDir(outputDir string) string {
	// For shows, create a show-specific folder
	showName, _ := filenamify.Filenamify(s.Results.Data.ShowName, filenamify.Options{MaxLength: 1000})
	return path.Join(outputDir, "Podcasts", showName)
}

func (s *Show) Unmarshal(data []byte) error {
	return json.Unmarshal(data, s)
}

// ToSong converts an Episode to a Song for compatibility with existing download logic
func (e *Episode) ToSong() *Song {
	// Use episode image if available, otherwise use show art
	cover := e.EpisodeImageMD5
	if cover == "" {
		cover = e.ShowArtMD5
	}

	return &Song{
		ID:          e.EpisodeID,
		Title:       e.EpisodeTitle,
		Artist:      e.ShowName,
		AlbumTitle:  e.ShowName,
		Duration:    e.Duration,
		TrackToken:  e.TrackToken,
		Cover:       cover,
		TrackNumber: "", // Episodes don't have track numbers
		EpisodeDirectStreamURL: e.EpisodeDirectStreamURL, // Store the direct stream URL
	}
}

func (e *Episode) GetTitle() string {
	return e.EpisodeTitle
}

// GetOrganizedPath returns the path for this episode: Podcasts/ShowName/EpisodeTitle
func (e *Episode) GetOrganizedPath(baseOutputDir string, media *Media) string {
	ext := "mp3"
	if len(media.Data) > 0 && len(media.Data[0].Media) > 0 && media.Data[0].Media[0].Format == "FLAC" {
		ext = "flac"
	}

	// Parse published date to add to filename for better organization
	publishedTime := ""
	if e.EpisodePublishedTS > 0 {
		t := time.Unix(e.EpisodePublishedTS, 0)
		publishedTime = t.Format("2006-01-02") + " - "
	}

	fileName := fmt.Sprintf("%s%s.%s", publishedTime, e.EpisodeTitle, ext)

	// Sanitize all path components
	showName, _ := filenamify.Filenamify(e.ShowName, filenamify.Options{MaxLength: 1000})
	fileName, _ = filenamify.Filenamify(fileName, filenamify.Options{MaxLength: 1000})

	return path.Join(baseOutputDir, "Podcasts", showName, fileName)
}
