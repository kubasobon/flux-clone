package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"

	"github.com/fluxcd/pkg/untar"
	flag "github.com/spf13/pflag"
)

var (
	localPort        int
	serviceName      string
	serviceNamespace string
	servicePort      int
	revision         string

	repoName      string
	repoNamespace string
)

func main() {
	flag.IntVar(&localPort, "local-port", 8080, "local port for port-forward")
	flag.StringVar(&repoName, "repo-name", "", "GitRepository name")
	flag.StringVar(&repoNamespace, "repo-namespace", "flux-system", "GitRepository namespace")
	flag.StringVar(&revision, "revision", "latest", "GitRepository revision")
	flag.StringVar(&serviceName, "service-name", "source-controller", "service name")
	flag.StringVar(&serviceNamespace, "service-namespace", "flux-system", "service namespace")
	flag.IntVar(&servicePort, "service-port", 80, "service port for port-forward")
	flag.Parse()

	if repoName == "" || repoNamespace == "" {
		log.Fatal("--repo-name and --repo-namespace flags are mandatory")
	}

	// Start port forwarding
	var wg sync.WaitGroup
	interrupt := make(chan os.Signal, 1)
	{
		log.Printf(
			"Starting port forwarding to %s/%s %d:%d...",
			serviceNamespace, serviceName, localPort, servicePort,
		)
		signal.Notify(interrupt, os.Interrupt)

		cmd := exec.Command(
			"kubectl",
			"port-forward",
			"-n",
			serviceNamespace,
			fmt.Sprintf("svc/%s", serviceName),
			fmt.Sprintf("%d:%d", localPort, servicePort),
		)
		wg.Add(1)
		go func() {
			err := cmd.Start()
			if err != nil {
				log.Fatal(err)
			}
			s := <-interrupt
			err = cmd.Process.Signal(s)
			if err != nil {
				log.Fatal(err)
			}
			log.Println("Ended port forwarding")
			wg.Done()
		}()
	}
	// wait a second for port-forwarding to be established
	time.Sleep(3 * time.Second)

	// download and untar the artifact
	dir, err := os.MkdirTemp(
		"",
		fmt.Sprintf("%s-%s-%s-*", repoNamespace, repoName, revision),
	)
	if err != nil {
		interrupt <- os.Interrupt
		wg.Wait()
		log.Fatal(err)
	}

	log.Println("Downloading and untarring the repo...")

	err = downloadRepo(dir)
	if err != nil {
		interrupt <- os.Interrupt
		wg.Wait()
		log.Fatal(err)
	}

	// end port forwarding
	interrupt <- os.Interrupt
	wg.Wait()
}

func downloadRepo(dir string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	request, err := http.NewRequest(
		"GET",
		fmt.Sprintf(
			"http://localhost:%d/gitrepository/%s/%s/%s.tar.gz",
			localPort, repoNamespace, repoName, revision,
		),
		nil,
	)
	if err != nil {
		return err
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"error calling %q: expected %d, got %d",
			request.URL, http.StatusOK, response.StatusCode,
		)
	}
	defer response.Body.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, response.Body)
	if err != nil {
		return err
	}
	log.Printf("Downloaded %q", request.URL)

	if _, err = untar.Untar(&buf, dir); err != nil {
		return err
	}
	log.Printf("Untarred in %q", dir)

	return nil
}
