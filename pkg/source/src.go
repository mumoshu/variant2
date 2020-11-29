package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fluxcd/pkg/untar"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	"golang.org/x/xerrors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	Client client.Client
}

// FetchTarball returns an io.ReadCloser that contains the http response body on successful request.
// It's the user's responsibility to close any non-nil ReadCloser otherwise the
// original http.Response.Body leaks.
func (c *Client) FetchTarball(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request for %s, error: %w", url, err)
	}

	//nolint:bodyclose
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact from %s, error: %w", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artifact '%s' download failed (status code: %s)", url, resp.Status)
	}

	return resp.Body, nil
}

func (c *Client) DownloadTarball(tarballURL string, f io.Writer) error {
	body, err := c.FetchTarball(context.Background(), tarballURL)

	defer func() {
		if body != nil {
			_ = body.Close()
		}
	}()

	if err != nil {
		return xerrors.Errorf("fetching tarball: %w", err)
	}

	if _, err = io.Copy(f, body); err != nil {
		return err
	}

	return nil
}

func (c *Client) ExtractTarball(url, dir string) error {
	timeout := time.Second * 30
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	defer cancel()

	body, err := c.FetchTarball(ctx, url)
	if err != nil {
		return xerrors.Errorf("fetching %s: %w", url, err)
	}

	defer func() {
		if body != nil {
			_ = body.Close()
		}
	}()

	if _, err = untar.Untar(body, dir); err != nil {
		return fmt.Errorf("faild to untar artifact, error: %w", err)
	}

	return nil
}

type TransientError struct {
	err error
}

func (e *TransientError) Error() string {
	return e.err.Error()
}

func (e *TransientError) Unwrap() error {
	return e.err
}

func (c *Client) ExtractSource(ctx context.Context, kind, ns, name string) (string, error) {
	// resolve source reference
	source, err := c.getSource(ctx, kind, ns, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := "Source not found"
			// do not requeue on this error, when the artifact is created the watcher should trigger a reconciliation
			return "", xerrors.Errorf("getting artifact: %s: %w", msg, err)
		}

		// retry on transient errors
		return "", xerrors.Errorf("getting artifact: %w", &TransientError{err})
	}

	if source.GetArtifact() == nil {
		msg := "Source is not ready, artifact not found"
		// do not requeue on this error, when the artifact is created the watcher should trigger a reconciliation
		return "", fmt.Errorf("getting artifact: %s", msg)
	}

	artifact := source.GetArtifact()
	tarballURL := artifact.URL

	var dir string

	if artifact.Revision != "" {
		dir = filepath.Join(os.TempDir(), "variant", "cache", "source", fmt.Sprintf("%s-%s-%s-%s", kind, ns, name, artifact.Revision))
	} else {
		dir = filepath.Join(os.TempDir(), "variant", "cache", "source", fmt.Sprintf("%s-%s-%s", kind, ns, name))
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", xerrors.Errorf("creating source cache dir: %w", err)
	}

	if info, err := os.Stat(dir); info == nil {
		return "", xerrors.Errorf("looking for source cache: %w", err)
	}

	extErr := c.ExtractTarball(tarballURL, dir)

	if extErr != nil {
		return "", xerrors.Errorf("extracting tarball: %w", extErr)
	}

	return dir, nil
}

func (c *Client) getSource(ctx context.Context, kind, namespace, name string) (sourcev1.Source, error) {
	var source sourcev1.Source

	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	if kind == "" || kind == "git" {
		kind = sourcev1.GitRepositoryKind
	}

	switch kind {
	case sourcev1.GitRepositoryKind:
		var repository sourcev1.GitRepository

		err := c.Client.Get(ctx, namespacedName, &repository)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return source, err
			}

			return source, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}

		source = &repository
	case sourcev1.BucketKind:
		var bucket sourcev1.Bucket

		err := c.Client.Get(ctx, namespacedName, &bucket)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return source, err
			}

			return source, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}

		source = &bucket
	default:
		return source, fmt.Errorf("source `%s` kind '%s' not supported",
			name, kind)
	}

	return source, nil
}
