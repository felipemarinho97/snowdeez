package deezer

import (
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/flytam/filenamify"
)

// Talk represents an individual podcast episode that can be fetched directly
type Talk struct {
	Results struct {
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
	} `json:"results"`
}

func (t *Talk) String() string {
	duration := "Unknown"
	if t.Results.Duration != "" {
		duration = t.Results.Duration + "s"
	}

	publishedDate := "Unknown"
	if t.Results.EpisodePublishedTS > 0 {
		publishedDate = time.Unix(t.Results.EpisodePublishedTS, 0).Format("2006-01-02")
	}

	return fmt.Sprintf(
		`=============== [ Episode Info ] ===============
Title:       %s
Show:        %s
Duration:    %s
Published:   %s
Description: %s
===============================================`,
		t.Results.EpisodeTitle,
		t.Results.ShowName,
		duration,
		publishedDate,
		t.Results.EpisodeDescription[:min(100, len(t.Results.EpisodeDescription))],
	)
}

func (t *Talk) GetType() string {
	return "Episode"  // Use episode.getData endpoint
}

func (t *Talk) GetTitle() string {
	return t.Results.EpisodeTitle
}

func (t *Talk) GetSongs() []*Song {
	// Convert the single episode to a song
	cover := t.Results.EpisodeImageMD5
	if cover == "" {
		cover = t.Results.ShowArtMD5
	}

	song := &Song{
		ID:          t.Results.EpisodeID,
		Title:       t.Results.EpisodeTitle,
		Artist:      t.Results.ShowName,
		AlbumTitle:  t.Results.ShowName,
		Duration:    t.Results.Duration,
		TrackToken:  t.Results.TrackToken,
		Cover:       cover,
		TrackNumber: "", // Episodes don't have track numbers
		EpisodeDirectStreamURL: t.Results.EpisodeDirectStreamURL,
	}
	return []*Song{song}
}

func (t *Talk) SetSongs(songs []*Song) {
	// For a single episode, we just update our data with the first song
	if len(songs) > 0 {
		song := songs[0]
		t.Results.EpisodeID = song.ID
		t.Results.EpisodeTitle = song.Title
		t.Results.Duration = song.Duration
		t.Results.ShowName = song.Artist
		t.Results.TrackToken = song.TrackToken
	}
}

func (t *Talk) GetOutputDir(outputDir string) string {
	// For individual episodes, create show-specific folder
	showName, _ := filenamify.Filenamify(t.Results.ShowName, filenamify.Options{MaxLength: 1000})
	return path.Join(outputDir, "Podcasts", showName)
}

func (t *Talk) Unmarshal(data []byte) error {
	return json.Unmarshal(data, t)
}
