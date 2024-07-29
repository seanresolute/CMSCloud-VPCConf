package common

import (
	"log"
	"os"
)

func GetRequiredEnvironmentVariable(envVar string) string {
	varStr := os.Getenv(envVar)
	if varStr == "" {
		log.Fatalf("ERROR: Required environment not set: %s", envVar)
		os.Exit(3)
	}
	return varStr
}
