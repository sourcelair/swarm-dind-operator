# Docker Swarm DinD Operator

Simple operator for running Docker in Docker (DinD) as a Swarm service

## Configuration

The following flags are currently available:

* `--args string` - the arguments to give to the DinD container
* `--binds stringArray` - the directories to bind in the container
* `--image string` - the image to use for DinD (default "docker:18.09-dind")
* `--name string` - the name to give to the DinD container (default "swarm-dind-operator")
* `--stop-timeout int` - the timeout to wait when the container is stopped in seconds (default 10)
* `--help` prints the help message

## Example usage

Given the following `docker-compose.yml` file

```yaml
version: '3.7'

services:
  dind:
    image: sourcelair/swarm-operator-dind
    command:
    - swarm-operator-dind
    - --args=--experimental
    - --binds=/mnt/dind:/var/lib/docker
    volumes:
	  - /var/run/docker.sock:/var/run/docker.sock
```

you can deploy the operator to Docker Swarm with

```bash
docker stack deploy -c docker-compose.yml
```
