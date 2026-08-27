package domain

import "sort"

// CanonicalizeManifest returns a copy with stable ordering for hashing and
// audit rendering. The input slices are never mutated.
func CanonicalizeManifest(manifest ReleaseManifest) ReleaseManifest {
	copyManifest := manifest
	copyManifest.Segments = append([]RecordingSegment(nil), manifest.Segments...)
	copyManifest.Annotations = append([]TranscriptAnnotation(nil), manifest.Annotations...)
	sort.SliceStable(copyManifest.Segments, func(i, j int) bool { return copyManifest.Segments[i].ID < copyManifest.Segments[j].ID })
	sort.SliceStable(copyManifest.Annotations, func(i, j int) bool {
		if copyManifest.Annotations[i].SegmentID != copyManifest.Annotations[j].SegmentID {
			return copyManifest.Annotations[i].SegmentID < copyManifest.Annotations[j].SegmentID
		}
		return copyManifest.Annotations[i].Revision < copyManifest.Annotations[j].Revision
	})
	return copyManifest
}
