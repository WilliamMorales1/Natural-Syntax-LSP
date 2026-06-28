package main

// Predictor abstracts POS tagging and semantic embedding modes.
type Predictor interface {
	Predict(text string) ([]POSToken, error)
	Close()
}
