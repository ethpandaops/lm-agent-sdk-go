package userinput

import "context"

// QuestionOption represents a selectable choice in a user-input question.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Question represents a single user-input prompt.
type Question struct {
	ID       string           `json:"id"`
	Header   string           `json:"header,omitempty"`
	Question string           `json:"question"`
	IsOther  bool             `json:"is_other,omitempty"`
	IsSecret bool             `json:"is_secret,omitempty"`
	Options  []QuestionOption `json:"options,omitempty"`
}

// Answer contains the user's response(s) to a single question.
type Answer struct {
	Answers []string `json:"answers"`
}

// Request contains parsed user-input payload data.
type Request struct {
	ItemID    string     `json:"item_id,omitempty"`
	ThreadID  string     `json:"thread_id,omitempty"`
	TurnID    string     `json:"turn_id,omitempty"`
	Questions []Question `json:"questions"`
}

// Response contains answers keyed by question ID.
type Response struct {
	Answers map[string]*Answer `json:"answers"`
}

// Callback handles user-input requests and returns answers.
type Callback func(ctx context.Context, req *Request) (*Response, error)
