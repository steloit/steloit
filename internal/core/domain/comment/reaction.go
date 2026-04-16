package comment

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

type Reaction struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	CommentID uuid.UUID `json:"comment_id" gorm:"type:uuid;not null"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;not null"`
	Emoji     string    `json:"emoji" gorm:"type:varchar(8);not null"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;autoCreateTime"`
}

func (Reaction) TableName() string {
	return "comment_reactions"
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
