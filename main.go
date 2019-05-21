package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func getOpt(opt string, args []string) ([]string, bool) {
	v1 := "--" + opt
	v2 := v1 + "="

	for i, arg := range args {
		if strings.HasPrefix(arg, v2) {
			return strings.Split(arg[len(v2):], " "), true
		}

		if strings.HasPrefix(arg, v1) {
			var vals []string
			for j := i + 1; j < len(args); j++ {
				if strings.HasPrefix(args[j], "--") {
					break
				}
				vals = append(vals, args[j])
			}
			return vals, true
		}
	}

	return nil, false
}

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

		var region string
		var registry []string

		if len(args) > 2 {
			if v, ok := getOpt("region", args[2:]); ok && len(v) == 1 {
				region = v[0]
			}

			if v, ok := getOpt("registry-ids", args[2:]); ok {
				registry = v
			}
		}

		getEcrLogin(region, registry)
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
		}).Send(context.Background())
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

	if isS3Path(dstPath) {
		bucket, key := parseS3Path(dstPath)
		reader, ok := src.(io.ReadSeeker)
		if !ok {
			panic("cannot cp s3 -> s3")
		}
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
		}).Send(context.Background())
		if err != nil {
			panic(err)
		}
	} else if dstPath == "-" {
		if _, err := io.Copy(os.Stdout, src); err != nil {
			panic(err)
		}
	} else {
		if dstPath == "." {
			parts := strings.Split(srcPath, "/")
			dstPath = parts[len(parts)-1]
		}

		writer, err := os.Create(dstPath)
		if err != nil {
			panic(err)
		}

		if _, err := io.Copy(writer, src); err != nil {
			panic(err)
		}
	}
}

func do(rc io.ReadCloser, f *os.File) {
	writer := bufio.NewWriter(f)
	buf := make([]byte, 4096>>2)
	for {
		nr, err := rc.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}

		_, err = writer.Write(buf[0:nr])
		if err != nil {
			panic(err)
		}
	}
	rc.Close()
	writer.Flush()
	f.Close()
}

func getEcrLogin(region string, registry []string) {
	cfg, err := external.LoadDefaultAWSConfig(&aws.Config{
		Region: region,
	})

	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	if region != "" {
		cfg.Region = region
	}

	client := ecr.New(cfg)
	out, err := client.GetAuthorizationTokenRequest(&ecr.GetAuthorizationTokenInput{
		RegistryIds: registry,
	}).Send(context.Background())
	if err != nil {
		panic(err)
	}

	for i := range out.AuthorizationData {
		endpoint := aws.StringValue(out.AuthorizationData[i].ProxyEndpoint)
		token := aws.StringValue(out.AuthorizationData[i].AuthorizationToken)
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
}
