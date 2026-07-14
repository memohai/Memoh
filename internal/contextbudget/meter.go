package contextbudget

const bytesPerEstimatedToken = 4

// EstimateTokensForBytes applies the shared conservative byte-to-token meter.
func EstimateTokensForBytes(byteCount int) int {
	if byteCount <= 0 {
		return 0
	}
	return (byteCount-1)/bytesPerEstimatedToken + 1
}

// EstimateTextTokens meters UTF-8 text by its encoded byte length.
func EstimateTextTokens(text string) int {
	return EstimateTokensForBytes(len(text))
}
