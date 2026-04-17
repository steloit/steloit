package comment

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

type Reaction struct {
	ID        uuid.UUID `json:"id"`
	CommentID uuid.UUID `json:"comment_id"`
	UserID    uuid.UUID `json:"user_id"`
	Emoji     string    `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
}

func NewReaction(commentID, userID uuid.UUID, emoji string) *Reaction {
	return &Reaction{
		ID:        uid.New(),
		CommentID: commentID,
		UserID:    userID,
		Emoji:     emoji,
		CreatedAt: time.Now(),
	}
}

type ReactionSummary struct {
	Emoji   string   `json:"emoji"`
	Count   int      `json:"count"`
	Users   []string `json:"users"`    // User names who reacted with this emoji
	HasUser bool     `json:"has_user"` // Whether the current user has reacted with this emoji
}

type ToggleReactionRequest struct {
	Emoji string `json:"emoji" binding:"required,max=8"`
}

const (
	MaxEmojisPerComment = 6 // Maximum different emoji types allowed per comment
)
