package layout

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"

	"github.com/buildpacks/imgutil"
)

func (i *Image) Save(additionalNames ...string) error {
	return i.SaveAs(i.Name(), additionalNames...)
}

// SaveAs ignores the image `Name()` method and saves the image according to name & additional names provided to this method
func (i *Image) SaveAs(name string, additionalNames ...string) error {
	if !i.preserveDigest {
		if err := i.SetCreatedAtAndHistory(); err != nil {
			return err
		}
	}

	refName, err := i.GetAnnotateRefName()
	if err != nil {
		return err
	}
	ops := []AppendOption{WithAnnotations(ImageRefAnnotation(refName))}
	if i.saveWithoutLayers {
		ops = append(ops, WithoutLayers())
	}

	if !i.preserveDigest {
		i.Image, err = imgutil.MutateManifest(i.Image, func(mfest *v1.Manifest) (mutateSubject, mutateAnnotations bool) {
			i.mutex.TryLock()
			defer i.mutex.Unlock()
			var (
				os, _          = i.OS()
				arch, _        = i.Architecture()
				variant, _     = i.Variant()
				osVersion, _   = i.OSVersion()
				features, _    = i.Features()
				osFeatures, _  = i.OSFeatures()
				annotations, _ = i.Annotations()
			)

			return imgutil.MutateManifestFn(mfest, os, arch, variant, osVersion, features, osFeatures, annotations)
		})
		if err != nil {
			return err
		}
	}

	var (
		pathsToSave = append([]string{name}, additionalNames...)
		diagnostics []imgutil.SaveDiagnostic
	)
	for _, path := range pathsToSave {
		layoutPath, err := initEmptyIndexAt(path)
		if err != nil {
			return err
		}
		if err = layoutPath.AppendImage(
			i.Image,
			ops...,
		); err != nil {
			diagnostics = append(diagnostics, imgutil.SaveDiagnostic{ImageName: i.Name(), Cause: err})
		}
	}
	if len(diagnostics) > 0 {
		return imgutil.SaveError{Errors: diagnostics}
	}

	return nil
}

func initEmptyIndexAt(path string) (Path, error) {
	return Write(path, empty.Index)
}
