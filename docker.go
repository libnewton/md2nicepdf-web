package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const (
	containerTimeout = 50 * time.Second
	imagePullTimeout = 5 * time.Minute
	cpuLimit         = 1_000_000_000      // 1 CPU in NanoCPUs
	memoryLimit      = 1024 * 1024 * 1024 // 1 GB
	storageSize      = "10G"
)

// CompilationError represents a document compilation failure reported by the
// converter itself. These messages are safe to expose to the user.
type CompilationError struct {
	Msg string
}

func (e *CompilationError) Error() string { return e.Msg }

func (s *Server) newDockerClient() (*client.Client, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(s.dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}

	return cli, nil
}

func (s *Server) ensureConverterImage(ctx context.Context) error {
	cli, err := s.newDockerClient()
	if err != nil {
		return err
	}
	defer cli.Close()

	if _, err := cli.ImageInspect(ctx, s.converterImage); err == nil {
		return nil
	} else if !errdefs.IsNotFound(err) {
		return fmt.Errorf("inspect image %q: %w", s.converterImage, err)
	}

	log.Printf("Converter image %s not found locally, pulling", s.converterImage)

	pullResp, err := cli.ImagePull(ctx, s.converterImage, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %q: %w", s.converterImage, err)
	}
	defer pullResp.Close()

	if err := pullResp.Wait(ctx); err != nil {
		return fmt.Errorf("wait for image pull %q: %w", s.converterImage, err)
	}

	log.Printf("Converter image %s pulled successfully", s.converterImage)
	return nil
}

// convertMarkdownToPDF orchestrates the full container lifecycle for converting
// markdown to PDF. It creates ephemeral directories, runs the converter
// container, and returns the resulting PDF bytes.
func (s *Server) convertMarkdownToPDF(markdown string, noPageNumbers bool) ([]byte, error) {
	ctx := context.Background()

	cli, err := s.newDockerClient()
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	// Create ephemeral temp directories under workDir so they are visible
	// to the host Docker daemon (the path must match on both sides of the
	// bind-mount when running inside a container).
	dataDir, err := os.MkdirTemp(s.workDir, "md2pdf-data-*")
	if err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	defer os.RemoveAll(dataDir)

	outDir, err := os.MkdirTemp(s.workDir, "md2pdf-out-*")
	if err != nil {
		return nil, fmt.Errorf("create out dir: %w", err)
	}
	defer os.RemoveAll(outDir)

	// Make dirs world-writable so the container (possibly running as non-root) can write
	os.Chmod(dataDir, 0o777)
	os.Chmod(outDir, 0o777)

	// Write input markdown
	inputPath := filepath.Join(dataDir, "file.md")
	if err := os.WriteFile(inputPath, []byte(markdown), 0o644); err != nil {
		return nil, fmt.Errorf("write input.md: %w", err)
	}

	// Create container
	containerName := fmt.Sprintf("md2pdf-job-%d", time.Now().UnixNano())

	var envVars []string
	if noPageNumbers {
		envVars = append(envVars, "NO_PAGE_NUMBERS=1")
	}

	containerCfg := &container.Config{
		Image: s.converterImage,
		Env:   envVars,
		Labels: map[string]string{
			"md2pdf": "ephemeral",
		},
	}

	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   dataDir,
				Target:   "/data",
				ReadOnly: true,
			},
			{
				Type:   mount.TypeBind,
				Source: outDir,
				Target: "/out",
			},
		},
		Resources: container.Resources{
			NanoCPUs: cpuLimit,
			Memory:   memoryLimit,
		},
		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges"},
		StorageOpt:  map[string]string{"size": storageSize},
		AutoRemove:  true,
	}

	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     containerCfg,
		HostConfig: hostCfg,
		Name:       containerName,
	})
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}

	containerID := resp.ID

	defer func() {
		// Force remove the container if it's still around (AutoRemove handles success cases)
		removeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cli.ContainerRemove(removeCtx, containerID, client.ContainerRemoveOptions{Force: true})
	}()

	if _, err := cli.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}

	// Wait for container with timeout
	waitCtx, waitCancel := context.WithTimeout(ctx, containerTimeout)
	defer waitCancel()

	waitResult := cli.ContainerWait(waitCtx, containerID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})

	select {
	case err := <-waitResult.Error:
		if err != nil {
			log.Printf("Container wait error (killing): %v", err)
			killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer killCancel()
			cli.ContainerKill(killCtx, containerID, client.ContainerKillOptions{Signal: "KILL"})
			return nil, fmt.Errorf("container error: %w", err)
		}
	case status := <-waitResult.Result:
		if status.StatusCode != 0 {
			logOutput := getContainerLogs(cli, containerID)
			return nil, &CompilationError{Msg: fmt.Sprintf("container exited with code %d: %s", status.StatusCode, logOutput)}
		}
	case <-waitCtx.Done():
		log.Printf("Container timed out after %v, killing", containerTimeout)
		killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer killCancel()
		cli.ContainerKill(killCtx, containerID, client.ContainerKillOptions{Signal: "KILL"})
		return nil, fmt.Errorf("conversion timed out after %v", containerTimeout)
	}

	// Read output PDF
	outputPath := filepath.Join(outDir, "file.pdf")
	pdfData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read output PDF: %w", err)
	}

	if len(pdfData) == 0 {
		return nil, fmt.Errorf("conversion produced an empty PDF")
	}

	log.Printf("PDF generated successfully (%d bytes)", len(pdfData))
	return pdfData, nil
}

// getContainerLogs retrieves the last portion of container logs for error reporting.
func getContainerLogs(cli *client.Client, containerID string) string {
	logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reader, err := cli.ContainerLogs(logCtx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "50",
	})
	if err != nil {
		return fmt.Sprintf("(failed to get logs: %v)", err)
	}
	defer reader.Close()

	logBytes, _ := io.ReadAll(reader)
	return string(logBytes)
}
