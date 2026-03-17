package main

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestMessageHasCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		entity  models.MessageEntity
		command string
		want    bool
	}{
		{
			name:    "plain command",
			text:    "/help",
			entity:  models.MessageEntity{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 5},
			command: "help",
			want:    true,
		},
		{
			name:    "command with args",
			text:    "/transactions 25",
			entity:  models.MessageEntity{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 13},
			command: "transactions",
			want:    true,
		},
		{
			name:    "command with bot username",
			text:    "/apicheck@wavy_vpn_bot",
			entity:  models.MessageEntity{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 22},
			command: "apicheck",
			want:    true,
		},
		{
			name:    "different command",
			text:    "/help",
			entity:  models.MessageEntity{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 5},
			command: "sync",
			want:    false,
		},
		{
			name:    "non leading command ignored",
			text:    "hi /help",
			entity:  models.MessageEntity{Type: models.MessageEntityTypeBotCommand, Offset: 3, Length: 5},
			command: "help",
			want:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			update := &models.Update{
				Message: &models.Message{
					Text:     tc.text,
					Entities: []models.MessageEntity{tc.entity},
				},
			}

			got := messageHasCommand(update, tc.command)
			if got != tc.want {
				t.Fatalf("messageHasCommand(%q, %q) = %v, want %v", tc.text, tc.command, got, tc.want)
			}
		})
	}
}
