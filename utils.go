// Utility functions.

package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func atoi(sval string) int {
	ival, err := strconv.ParseInt(strings.TrimSpace(sval), 10, 32)
	if err != nil {
		ival = 0
	}
	return int(ival)
}

func sha1string(path string) string {
	// Open the current executable file
	file, err := os.Open(path)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	defer file.Close()

	// Compute the SHA1 hash of the file
	hash := sha1.New()
	if _, err := io.Copy(hash, file); err != nil {
		fmt.Println("Error:", err)
		return ""
	}

	// Get the hash sum as a byte slice
	hashSum := hash.Sum(nil)

	// Convert the byte slice to a hex string
	return hex.EncodeToString(hashSum)
}
