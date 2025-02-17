package storage

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"
	"github.com/reedchan7/distribution"
	dcontext "github.com/reedchan7/distribution/context"
	"github.com/reedchan7/distribution/manifest/manifestlist"
)

// manifestListHandler is a ManifestHandler that covers schema2 manifest lists.
type manifestListHandler struct {
	repository distribution.Repository
	blobStore  distribution.BlobStore
	ctx        context.Context
}

var _ ManifestHandler = &manifestListHandler{}

func (ms *manifestListHandler) Unmarshal(ctx context.Context, dgst digest.Digest, content []byte) (distribution.Manifest, error) {
	dcontext.GetLogger(ms.ctx).Debug("(*manifestListHandler).Unmarshal")

	m := &manifestlist.DeserializedManifestList{}
	if err := m.UnmarshalJSON(content); err != nil {
		return nil, err
	}

	return m, nil
}

func (ms *manifestListHandler) Put(ctx context.Context, manifestList distribution.Manifest, skipDependencyVerification bool) (digest.Digest, error) {
	dcontext.GetLogger(ms.ctx).Debug("(*manifestListHandler).Put")

	m, ok := manifestList.(*manifestlist.DeserializedManifestList)
	if !ok {
		return "", fmt.Errorf("wrong type put to manifestListHandler: %T", manifestList)
	}

	if err := ms.verifyManifest(ms.ctx, *m, skipDependencyVerification); err != nil {
		return "", err
	}

	mt, payload, err := m.Payload()
	if err != nil {
		return "", err
	}

	revision, err := ms.blobStore.Put(ctx, mt, payload)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("error putting payload into blobstore: %v", err)
		return "", err
	}

	return revision.Digest, nil
}

// verifyManifest ensures that the manifest content is valid from the
// perspective of the registry. As a policy, the registry only tries to
// store valid content, leaving trust policies of that content up to
// consumers.
func (ms *manifestListHandler) verifyManifest(ctx context.Context, mnfst manifestlist.DeserializedManifestList, skipDependencyVerification bool) error {
	var errs distribution.ErrManifestVerification

	if mnfst.SchemaVersion != 2 {
		return fmt.Errorf("unrecognized manifest list schema version %d", mnfst.SchemaVersion)
	}

	if !skipDependencyVerification {
		// This manifest service is different from the blob service
		// returned by Blob. It uses a linked blob store to ensure that
		// only manifests are accessible.

		manifestService, err := ms.repository.Manifests(ctx)
		if err != nil {
			return err
		}

		for _, manifestDescriptor := range mnfst.References() {
			exists, err := manifestService.Exists(ctx, manifestDescriptor.Digest)
			if err != nil && err != distribution.ErrBlobUnknown {
				errs = append(errs, err)
			}
			if err != nil || !exists {
				// On error here, we always append unknown blob errors.
				errs = append(errs, distribution.ErrManifestBlobUnknown{Digest: manifestDescriptor.Digest})
			}
		}
	}
	if len(errs) != 0 {
		return errs
	}

	return nil
}
