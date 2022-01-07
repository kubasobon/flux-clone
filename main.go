package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/fluxcd/pkg/untar"
	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	localPort        int
	serviceName      string
	serviceNamespace string
	servicePort      int
	revision         string

	sourceType      string
	sourceName      string
	sourceNamespace string

	allowedTypes = []string{"gitrepository", "helmchart"}
)

func main() {
	allowedTypesString := strings.Join(allowedTypes, ", ")
	flag.IntVar(&localPort, "local-port", 8080, "local port for port-forward")
	flag.StringVar(&sourceName, "name", "", "Source name")
	flag.StringVar(&sourceNamespace, "namespace", "flux-system", "Source namespace")
	flag.StringVar(&revision, "revision", "latest", "Source revision")
	flag.StringVar(&serviceName, "service-name", "source-controller", "service name")
	flag.StringVar(&serviceNamespace, "service-namespace", "flux-system", "service namespace")
	flag.IntVar(&servicePort, "service-port", 80, "service port for port-forward")
	flag.StringVar(&sourceType, "source-type", "gitrepository", "type of source to use: "+allowedTypesString)
	flag.Parse()

	if !contains(allowedTypes, sourceType) {
		log.Fatalf("--source-type %q not allowed, must be one of: %s", sourceType, allowedTypesString)
	}
	if sourceName == "" || sourceNamespace == "" {
		log.Fatal("--name and --namespace flags are mandatory")
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
		fmt.Sprintf("%s-%s-%s-*", sourceNamespace, sourceName, revision),
	)
	if err != nil {
		interrupt <- os.Interrupt
		wg.Wait()
		log.Fatal(err)
	}

	log.Println("Downloading and untarring the source...")

	var url string
	switch sourceType {
	case "gitrepository":
		url = gitrepositoryURL()
	case "helmchart":
		url, err = helmchartURL()
		if err != nil {
			log.Fatal(err)
		}
	}

	err = downloadSource(dir, url)
	if err != nil {
		interrupt <- os.Interrupt
		wg.Wait()
		log.Fatal(err)
	}

	// end port forwarding
	interrupt <- os.Interrupt
	wg.Wait()
}

func gitrepositoryURL() string {
	return fmt.Sprintf(
		"http://localhost:%d/%s/%s/%s/%s.tar.gz",
		localPort, sourceType, sourceNamespace, sourceName, revision,
	)
}

func helmchartURL() (string, error) {
	log.Println("Getting URL from the HelmChart...")
	c, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return "", err
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "source.toolkit.fluxcd.io",
		Kind:    "HelmChart",
		Version: "v1beta1",
	})

	err = c.Get(
		context.Background(),
		client.ObjectKey{
			Namespace: sourceNamespace,
			Name:      sourceName,
		},
		u,
	)
	if err != nil {
		return "", err
	}

	url, ok, err := unstructured.NestedString(u.Object, "status", "url")
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf(".status.url not set")
	}

	// get rid of in-cluster service address
	urlElements := strings.SplitN(url, "/", 4)
	url = fmt.Sprintf("http://localhost:%d/%s", localPort, urlElements[len(urlElements)-1])
	return url, nil
}

func downloadSource(dir, url string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	request, err := http.NewRequest("GET", url, nil)
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

func contains(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}
