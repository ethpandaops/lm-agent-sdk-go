package exampleutil

import (
	"fmt"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func DisplayMessage(msg sdk.Message) {
	switch m := msg.(type) {
	case *sdk.UserMessage:
		for _, block := range m.Content.Blocks() {
			switch b := block.(type) {
			case *sdk.TextBlock:
				fmt.Printf("User: %s\n", b.Text)
			case *sdk.InputImageBlock:
				fmt.Printf("User image: %s\n", b.ImageURL.URL)
			case *sdk.InputFileBlock:
				fmt.Printf("User file: %s\n", b.File.Filename)
			case *sdk.InputAudioBlock:
				fmt.Printf("User audio: %s\n", b.InputAudio.Format)
			case *sdk.InputVideoBlock:
				fmt.Printf("User video: %s\n", b.VideoURL.URL)
			}
		}
	case *sdk.AssistantMessage:
		for _, block := range m.Content {
			switch b := block.(type) {
			case *sdk.TextBlock:
				fmt.Printf("Assistant: %s\n", b.Text)
			case *sdk.ImageBlock:
				label := b.MediaType
				if label == "" {
					label = "image"
				}
				fmt.Printf("Assistant image: %s\n", label)
			case *sdk.ToolUseBlock:
				fmt.Printf("Tool use: %s(%v)\n", b.Name, b.Input)
			case *sdk.ToolResultBlock:
				fmt.Printf("Tool result: %s\n", b.ToolUseID)
			}
		}
	case *sdk.ResultMessage:
		fmt.Printf("Result subtype: %s\n", m.Subtype)
		if m.Result != nil {
			fmt.Printf("Result text: %s\n", *m.Result)
		}
		if m.TotalCostUSD != nil {
			fmt.Printf("Cost (USD): %.8f\n", *m.TotalCostUSD)
		}
	case *sdk.SystemMessage:
		// Skip noisy system init messages in examples.
	case *sdk.StreamEvent:
		if eventType, _ := m.Event["type"].(string); eventType != "" {
			fmt.Printf("Stream event: %s\n", eventType)
		}
	default:
		fmt.Printf("Message: %#v\n", m)
	}
}
