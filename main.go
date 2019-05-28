package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/kballard/go-shellquote"
	"github.com/spf13/pflag"

	"github.com/pkg/errors"
)

var imageName, containerName, containerArgs string
var containerStopTimeout int
var containerBinds []string

func main() {
	os.Exit(runOperator())
}

func runOperator() int {
	// parse arguments
	pflag.StringVar(&imageName, "image", "docker:18.09-dind", "the image to use for DinD")
	pflag.StringVar(&containerName, "name", "swarm-dind-operator", "the name to give to the DinD container")
	pflag.StringVar(&containerArgs, "args", "", "the arguments to give to the DinD container")
	pflag.IntVar(&containerStopTimeout, "stop-timeout", 10, "the timeout to wait when the container is stopped in seconds")
	pflag.StringArrayVar(&containerBinds, "binds", []string{}, "the directories to bind in the container")
	pflag.Parse()

	filteredBinds := make([]string, len(containerBinds))
	for _, bind := range containerBinds {
		if bind != "" {
			filteredBinds = append(filteredBinds, bind)
		}
	}

	args, err := shellquote.Split(containerArgs)
	if err != nil {
		fmt.Println(errors.Wrap(err, "invalid arguments given"))
		return 42
	}

	data, err := ioutil.ReadFile("/proc/self/cpuset")
	if err != nil {
		fmt.Println(errors.Wrap(err, "cannot read cgroup file"))
		return 42
	}
	parts := strings.Split(string(data), "/")
	selfID := strings.TrimSpace(parts[len(parts)-1])

	client, err := docker.NewClientFromEnv()
	if err != nil {
		fmt.Println(errors.Wrap(err, "cannot create client"))
		return 42
	}
	containers, err := client.ListContainers(docker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"name": []string{containerName},
		},
	})
	if err != nil {
		fmt.Println(errors.Wrap(err, "cannot list containers"))
		return 42
	}

	for _, container := range containers {
		if err := client.RemoveContainer(docker.RemoveContainerOptions{
			Force: true,
			ID:    container.ID,
		}); err != nil {
			fmt.Println(errors.Wrap(err, "cannot remove container"))
			return 42
		}
	}
	if _, err := client.InspectImage(imageName); err != nil {
		if err != docker.ErrNoSuchImage {
			fmt.Println(errors.Wrap(err, "cannot inspect image"))
			return 42
		}

		if err := client.PullImage(docker.PullImageOptions{
			Repository: imageName,
		}, docker.AuthConfiguration{}); err != nil {
			fmt.Println(errors.Wrap(err, "cannot pull image"))
			return 42
		}
	}
	container, err := client.CreateContainer(docker.CreateContainerOptions{
		Name: containerName,
		Config: &docker.Config{
			Cmd:   args,
			Image: imageName,
			Labels: map[string]string{
				"com.sourcelair.swarm-dind-operator":      "true",
				"com.sourcelair.swarm-dind-operator.name": containerName,
			},
			StopTimeout: containerStopTimeout,
		},
		HostConfig: &docker.HostConfig{
			Privileged:  true,
			Binds:       filteredBinds,
			NetworkMode: fmt.Sprintf("container:%s", selfID),
			PidMode:     fmt.Sprintf("container:%s", selfID),
		},
	})
	if err != nil {
		fmt.Println(errors.Wrap(err, "cannot create container"))
		return 42
	}

	if err := client.StartContainer(container.ID, nil); err != nil {
		fmt.Println(errors.Wrap(err, "cannot start container"))
		return 42
	}

	ctx, done := context.WithCancel(context.Background())
	defer done()

	// forward container logs
	go func() {
		client.Logs(docker.LogsOptions{
			Container:    container.ID,
			Stdout:       true,
			Stderr:       true,
			OutputStream: os.Stdout,
			ErrorStream:  os.Stderr,
			Context:      ctx,
			Follow:       true,
		})
	}()

	defer client.RemoveContainer(docker.RemoveContainerOptions{
		Force: true,
		ID:    container.ID,
	})

	// wait for container to exit
	containerExited := make(chan int, 1)
	go func() {
		exitCode, _ := client.WaitContainer(container.ID)
		containerExited <- exitCode
	}()

	// catch signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs)

	exitCode := 0

	for {
		select {
		// propagate signals
		case sig := <-sigs:
			client.KillContainer(docker.KillContainerOptions{
				ID:      container.ID,
				Context: ctx,
				Signal:  docker.Signal(sig.(syscall.Signal)),
			})
		// exit with same exit code
		case exitCode = <-containerExited:
			return exitCode
		}
	}
}
