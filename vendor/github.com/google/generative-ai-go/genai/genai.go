package genai

import (
	"context"

	"google.golang.org/api/option"
)

type Part interface {
	part()
}

type Text string

func (t Text) part() {}

type Blob struct {
	MIMEType string
	Data     []byte
}

func (b Blob) part() {}

type Content struct {
	Parts []Part
	Role  string
}

type Candidate struct {
	Content *Content
}

type GenerateContentResponse struct {
	Candidates []*Candidate
}

type GenerativeModel struct {
	Name             string
	ResponseMIMEType string
	Temperature      float32
	MaxOutputTokens  int32
}

func (m *GenerativeModel) SetTemperature(t float32) {
	m.Temperature = t
}

func (m *GenerativeModel) SetMaxOutputTokens(tokens int32) {
	m.MaxOutputTokens = tokens
}

func (m *GenerativeModel) GenerateContent(ctx context.Context, parts ...Part) (*GenerateContentResponse, error) {
	return &GenerateContentResponse{}, nil
}

type Client struct{}

func NewClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	return &Client{}, nil
}

func (c *Client) GenerativeModel(name string) *GenerativeModel {
	return &GenerativeModel{Name: name}
}

func (c *Client) Close() error {
	return nil
}
