package layout

import (
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/buildpacks/imgutil"
)

func NewImage(path string, ops ...ImageOption) (*Image, error) {
	options := &imgutil.ImageOptions{}
	for _, op := range ops {
		op(options)
	}
	options.Platform = processDefaultPlatformOption(options.Platform)

	var err error
	if options.PreviousImageRepoName != "" {
		options.PreviousImage, err = newImageFromPath(options.PreviousImageRepoName, options.Platform, imgutil.DefaultTypes)
		if err != nil {
			return nil, err
		}
	}

	if options.BaseImage == nil && options.BaseImageRepoName != "" { // options.BaseImage supersedes options.BaseImageRepoName
		options.BaseImage, err = newImageFromPath(options.BaseImageRepoName, options.Platform, imgutil.DefaultTypes)
		if err != nil {
			return nil, err
		}
	}
	options.MediaTypes = imgutil.GetPreferredMediaTypes(*options)
	if options.BaseImage != nil {
		options.BaseImage, err = imgutil.EnsureMediaTypes(options.BaseImage, options.MediaTypes) // FIXME: this can move into imgutil constructor
		if err != nil {
			return nil, err
		}
	}

	cnbImage, err := imgutil.NewCNBImage(*options)
	if err != nil {
		return nil, err
	}

	return &Image{
		CNBImageCore:      cnbImage,
		repoPath:          path,
		saveWithoutLayers: options.WithoutLayers,
	}, nil
}

func processDefaultPlatformOption(requestedPlatform imgutil.Platform) imgutil.Platform {
	var emptyPlatform imgutil.Platform
	if requestedPlatform != emptyPlatform {
		return requestedPlatform
	}
	return imgutil.Platform{
		OS:           "linux",
		Architecture: "amd64",
	}
}

// newImageFromPath creates a layout image from the given path.
// * If an image index for multiple platforms exists, it will try to select the image according to the platform provided.
// * If the image does not exist, then nothing is returned.
func newImageFromPath(path string, withPlatform imgutil.Platform, withMediaTypes imgutil.MediaTypes) (v1.Image, error) {
	if !imageExists(path) {
		return nil, nil
	}

	layoutPath, err := FromPath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load layout from path: %w", err)
	}
	index, err := layoutPath.ImageIndex()
	if err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	image, err := imageFromIndex(index, withPlatform)
	if err != nil {
		return nil, fmt.Errorf("failed to load image from index: %w", err)
	}

	// ensure layers will not error when accessed if there is no underlying data
	manifestFile, err := image.Manifest()
	if err != nil {
		return nil, err
	}
	configFile, err := image.ConfigFile()
	if err != nil {
		return nil, err
	}
	return imgutil.EnsureMediaTypesAndLayers(image, withMediaTypes, func(idx int, layer v1.Layer) (v1.Layer, error) {
		return newLayerOrFacadeFrom(*configFile, *manifestFile, idx, layer)
	})
}

// imageFromIndex creates a v1.Image from the given Image Index, selecting the image manifest
// that matches the given OS and architecture.
func imageFromIndex(index v1.ImageIndex, platform imgutil.Platform) (v1.Image, error) {
	manifestList, err := index.IndexManifest()
	if err != nil {
		return nil, err
	}
	if len(manifestList.Manifests) == 0 {
		return nil, fmt.Errorf("failed to find manifest at index")
	}

	// find manifest for platform
	var manifest v1.Descriptor
	if len(manifestList.Manifests) == 1 {
		manifest = manifestList.Manifests[0]
	} else {
		for _, m := range manifestList.Manifests {
			if m.Platform.OS == platform.OS &&
				m.Platform.Architecture == platform.Architecture {
				manifest = m
				break
			}
		}
		return nil, fmt.Errorf("failed to find manifest matching platform %v", platform)
	}

	return index.Image(manifest.Digest)
}
