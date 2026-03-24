package deezer

import (
	"encoding/json"
	"fmt"
	"time"
)

type EpisodeResource struct {
	Results struct {
		Data *Episode `json:"DATA"`
	} `json:"results"`
}

func (e *EpisodeResource) String() string {
	if e.Results.Data == nil {
		return "Episode: No data available"
	}

	duration := "Unknown"
	if e.Results.Data.Duration != "" {
		if d, err := time.ParseDuration(e.Results.Data.Duration + "s"); err == nil {
			duration = d.String()
		}
	}

	return fmt.Sprintf(
		`=============== [ Episode Info ] ===============
Title:       %s
Show:        %s
Duration:    %s
Description: %s
===============================================`,
		e.Results.Data.EpisodeTitle,
		e.Results.Data.ShowName,
		duration,
		e.Results.Data.EpisodeDescription[:min(100, len(e.Results.Data.EpisodeDescription))],
	)
}

func (e *EpisodeResource) GetType() string {
	return "Episode"
}

func (e *EpisodeResource) GetTitle() string {
	if e.Results.Data == nil {
		return ""
	}
	return e.Results.Data.EpisodeTitle
}

func (e *EpisodeResource) GetSongs() []*Song {
	if e.Results.Data == nil {
		return []*Song{}
	}
	return []*Song{e.Results.Data.ToSong()}
}

func (e *EpisodeResource) SetSongs(songs []*Song) {
	if len(songs) > 0 {
		e.Results.Data = &Episode{
			EpisodeID:    songs[0].ID,
			EpisodeTitle: songs[0].Title,
			Duration:     songs[0].Duration,
			ShowName:     songs[0].Artist,
			TrackToken:   songs[0].TrackToken,
			Available:    true,
			Type:         "episode",
		}
	}
}

func (e *EpisodeResource) GetOutputDir(outputDir string) string {
	// For single episodes, return the base output directory since episodes will handle their own paths
	return outputDir
}

func (e *EpisodeResource) Unmarshal(data []byte) error {
	return json.Unmarshal(data, e)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
