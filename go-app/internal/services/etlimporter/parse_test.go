package etlimporter

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseBattingCSV(t *testing.T) {
	tests := []struct {
		name    string
		csv     string
		wantErr bool
		wantN   int
	}{
		{
			name:    "empty csv",
			csv:     "",
			wantErr: true,
		},
		{
			name:    "bad header",
			csv:     "player,season\nA,2024\n",
			wantErr: true,
		},
		{
			name: "happy path",
			csv: strings.Join([]string{
				"player_name,season,format,runs,balls,fours,sixes,position",
				"Virat Kohli,2024,odi,100,90,8,2,3",
			}, "\n"),
			wantErr: false,
			wantN:   1,
		},
		{
			name:    "parse error in int",
			csv:     "player_name,season,format,runs,balls,fours,sixes,position\nA,2024,odi,xx,90,1,0,1\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := ParseBattingCSV(bytes.NewBufferString(tt.csv))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(rows) != tt.wantN {
				t.Fatalf("want %d rows, got %d", tt.wantN, len(rows))
			}
		})
	}
}

func TestParseBowlingCSV(t *testing.T) {
	tests := []struct {
		name    string
		csv     string
		wantErr bool
		wantN   int
	}{
		{
			name:    "empty csv",
			csv:     "",
			wantErr: true,
		},
		{
			name:    "bad header",
			csv:     "player,season\nA,2024\n",
			wantErr: true,
		},
		{
			name: "happy path",
			csv: strings.Join([]string{
				"player_name,season,format,overs,balls,maidens,runs,wickets,economy",
				"Jasprit Bumrah,2024,t20,4,24,1,20,3,5.0",
			}, "\n"),
			wantErr: false,
			wantN:   1,
		},
		{
			name:    "parse error in float",
			csv:     "player_name,season,format,overs,balls,maidens,runs,wickets,economy\nA,2024,t20,xx,24,0,20,2,6.0\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := ParseBowlingCSV(bytes.NewBufferString(tt.csv))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(rows) != tt.wantN {
				t.Fatalf("want %d rows, got %d", tt.wantN, len(rows))
			}
		})
	}
}
