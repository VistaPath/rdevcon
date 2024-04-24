// AWS setup and downloads, for access to keys and device list from S3.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

var s3svc *s3.S3

// s3Get fetches the S3 URI and returns []byte.
func s3Get(s3url string) ([]byte, error) {
	// With help from ChatGPT.

	if s3svc == nil {
		fmt.Println("*** AWS is not configured, S3 not available")
		fmt.Printf("*** cannot download %s\n", s3url)
		return nil, errors.New("AWS not configured")
	}

	parts := strings.SplitN(s3url[5:], "/", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid s3 URL")
	}

	bucketName, objectKey := parts[0], parts[1]

	params := &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	}

	resp, err := s3svc.GetObject(params)
	if err != nil {
		return nil, err
	} else {
		defer resp.Body.Close()
	}

	buffer := bytes.Buffer{}
	if _, err := buffer.ReadFrom(resp.Body); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// Simple wrapper around s3Get that saves to a file.
func s3Download(s3url string, localFilePath string) error {
	buffer, err := s3Get(s3url)
	if err != nil {
		return err
	}

	return os.WriteFile(localFilePath, buffer, 0600)
}

// Return the value for a key under an S3 object's Metadata.
func s3Metadata(s3url string, s3key string) (string, error) {
	if s3svc == nil {
		fmt.Println("*** AWS is not configured, S3 not available")
		return "", errors.New("AWS not configured")
	}

	parts := strings.SplitN(s3url[5:], "/", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid s3 URL")
	}

	bucketName, objectKey := parts[0], parts[1]
	// Create input parameters for the HeadObject operation
	params := &s3.HeadObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	}

	// Call the HeadObject operation to retrieve metadata
	resp, err := s3svc.HeadObject(params)
	if err != nil {
		fmt.Println("Error retrieving object metadata:", err)
		return "", err
	}

	// Search for the key in the Metadata
	for key, value := range resp.Metadata {
		if key == s3key {
			return *value, nil
		}
	}

	return "", errors.New("Metadata key not found")
}

func awsSetup() {
	defer fmt.Println("")

	for _, k := range []string{"AWS_SECRET_ACCESS_KEY", "AWS_ACCESS_KEY_ID", "AWS_SESSION_TOKEN"} {
		if os.Getenv(k) == "" {
			fmt.Println("AWS: environment not set, using built-in and cached defaults")
			return
		}
	}

	sess, err := session.NewSession(
		&aws.Config{
			Region: aws.String("us-east-1"),
		})

	if err != nil {
		fmt.Println("AWS: session failed, refresh your SSO session and environment vars")
		os.Exit(1)
	}

	s3svc = s3.New(sess)
}
