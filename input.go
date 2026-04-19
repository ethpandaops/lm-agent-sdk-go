package lmsdk

// Text creates text-only user content.
func Text(text string) UserMessageContent {
	return NewUserMessageContent(text)
}

// TextInput creates a text block for block-based multimodal user content.
func TextInput(text string) *TextBlock {
	return &TextBlock{
		Type: BlockTypeText,
		Text: text,
	}
}

// Blocks creates block-based user content.
func Blocks(blocks ...ContentBlock) UserMessageContent {
	return NewUserMessageContentBlocks(blocks)
}

// ImageInput creates an input image block from a URL or data URL.
func ImageInput(url string) *InputImageBlock {
	return &InputImageBlock{
		Type: BlockTypeImageURL,
		ImageURL: InputImageRef{
			URL: url,
		},
	}
}

// FileInput creates an input file block from a URL or data URL.
func FileInput(filename, fileData string) *InputFileBlock {
	return &InputFileBlock{
		Type: BlockTypeFile,
		File: InputFileRef{
			Filename: filename,
			FileData: fileData,
		},
	}
}

// AudioInput creates an input audio block from base64 audio plus its format.
func AudioInput(format, data string) *InputAudioBlock {
	return &InputAudioBlock{
		Type: BlockTypeInputAudio,
		InputAudio: InputAudioRef{
			Data:   data,
			Format: format,
		},
	}
}

// VideoInput creates an input video block from a URL or data URL.
func VideoInput(url string) *InputVideoBlock {
	return &InputVideoBlock{
		Type: BlockTypeVideoURL,
		VideoURL: InputVideoRef{
			URL: url,
		},
	}
}
