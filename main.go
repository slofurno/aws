package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	args := os.Args[1:]
	if len(args) < 1 {
		panic("missing command")
	}

	switch args[0] {
	case "s3":
		if len(args) < 4 || args[1] != "cp" {
			panic("s3 cp <src> <dst>")
		}
		s3Copy(args[2], args[3])

	case "ecr":
		//aws ecr get-login --registry-ids 012345678910 023456789012

		if len(args) < 2 || args[1] != "get-login" {
			panic("ecr get-login")
		}
		getEcrLogin()
	}
}

func isS3Path(path string) bool {
	return strings.Contains(path, "s3://")
}

func parseS3Path(path string) (string, string) {
	path = strings.TrimPrefix(path, "s3://")
	parts := strings.SplitN(path, "/", 2)
	return parts[0], parts[1]
}

func s3Copy(srcPath, dstPath string) {
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	svc := s3.New(cfg)
	var src io.ReadCloser

	if isS3Path(srcPath) {
		bucket, key := parseS3Path(srcPath)
		out, err := svc.GetObjectRequest(&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		}).Send()
		if err != nil {
			panic(err)
		}

		src = out.Body
	} else if srcPath == "-" {
		src = os.Stdin
	} else {
		var err error
		if src, err = os.Open(srcPath); err != nil {
			panic(err)
		}
	}

	b, err := ioutil.ReadAll(src)
	if err != nil {
		panic(err)
	}

	reader := bytes.NewReader(b)

	if isS3Path(dstPath) {
		bucket, key := parseS3Path(dstPath)
		/*
			svc := s3manager.NewUploader(cfg)
			_, err := svc.Upload(&s3manager.UploadInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
				Body:   src,
			})
		*/
		_, err := svc.PutObjectRequest(&s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   reader,
		}).Send()
		if err != nil {
			panic(err)
		}
	} else if dstPath == "-" {
		if _, err := io.Copy(os.Stdout, reader); err != nil {
			panic(err)
		}
	} else {
		writer, err := os.Create(dstPath)
		if err != nil {
			panic(err)
		}

		if _, err := io.Copy(writer, reader); err != nil {
			panic(err)
		}
	}
}

func getEcrLogin() {
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	client := ecr.New(cfg)
	out, err := client.GetAuthorizationTokenRequest(&ecr.GetAuthorizationTokenInput{}).Send()
	if err != nil {
		panic(err)
	}

	endpoint := aws.StringValue(out.AuthorizationData[0].ProxyEndpoint)
	token := aws.StringValue(out.AuthorizationData[0].AuthorizationToken)
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		panic(err)
	}

	split := strings.Split(string(decoded), ":")
	password := split[1]
	user := split[0]

	//"docker login -u AWS -p xxx -e none https://142221083342.dkr.ecr.us-east-1.amazonaws.com"
	fmt.Printf("docker login -u %s -p %s %s\n", user, password, endpoint)
}
