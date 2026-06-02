// SPDX-License-Identifier: Apache-2.0

// Mock OCI registry for integration testing.
// Serves embedded Gemara catalog and policy YAML as OCI artifacts
// via the OCI Distribution Spec v2 endpoints.
//
// Start: go run ./cmd/mock-oci-registry
// Usage: COMPLYCTL_REGISTRY_URL=http://localhost:8765 complytime get

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

//go:embed testdata/*.yaml
var seedData embed.FS

const defaultPort = "8765"

const (
	ociManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
	ociEmptyConfigType   = "application/vnd.oci.empty.v1+json"
	gemaraCatalogType    = "application/vnd.gemara.catalog.v1+yaml"
	gemaraPolicyType     = "application/vnd.gemara.policy.v1+yaml"
)

// ComplyPack media types — hardcoded to avoid importing the complypack package.
const (
	complypackArtifactType = "application/vnd.complypack.artifact.v1"
	complypackConfigType   = "application/vnd.complypack.config.v1+json"
	complypackContentType  = "application/vnd.complypack.content.v1.tar+gzip"
)

func main() {
	port := os.Getenv("GEMARA_SERVICE_PORT")
	if port == "" {
		port = defaultPort
	}

	store := newContentStore()
	store.seedDefaults()

	mux := http.NewServeMux()
	registerOCIRoutes(mux, store)

	addr := ":" + port
	log.Printf("mock-oci-registry listening on http://localhost%s", addr) //nolint:gosec // G706: addr is derived from env port with hardcoded fallback
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// registerOCIRoutes adds OCI Distribution Spec v2 endpoints.
func registerOCIRoutes(mux *http.ServeMux, store *contentStore) {
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v2/")
		path = strings.TrimSuffix(path, "/")

		// GET /v2/ — API version check
		if path == "" {
			w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
			return
		}

		// GET /v2/_catalog
		if path == "_catalog" {
			serveCatalog(w, store)
			return
		}

		// Route: /v2/{name}/tags/list
		if idx := strings.LastIndex(path, "/tags/list"); idx > 0 {
			repoName := path[:idx]
			serveTagsList(w, store, repoName)
			return
		}

		// Route: /v2/{name}/manifests/{reference}
		if idx := strings.LastIndex(path, "/manifests/"); idx > 0 {
			repoName := path[:idx]
			reference := path[idx+len("/manifests/"):]
			serveManifest(w, r, store, repoName, reference)
			return
		}

		// Route: /v2/{name}/blobs/{digest}
		if idx := strings.LastIndex(path, "/blobs/"); idx > 0 {
			repoName := path[:idx]
			digest := path[idx+len("/blobs/"):]
			serveBlob(w, store, repoName, digest)
			return
		}

		writeOCIError(w, http.StatusBadRequest, "NAME_UNKNOWN", "invalid API path")
	})
}

// serveCatalog handles GET /v2/_catalog
func serveCatalog(w http.ResponseWriter, store *contentStore) {
	repos := store.listRepositories()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"repositories": repos,
	})
}

// serveTagsList handles GET /v2/{name}/tags/list
func serveTagsList(w http.ResponseWriter, store *contentStore, repoName string) {
	repo, ok := store.repos[repoName]
	if !ok {
		writeOCIError(w, http.StatusNotFound, "NAME_UNKNOWN", fmt.Sprintf("repository %q not found", repoName))
		return
	}

	tags := make([]string, 0, len(repo.tags))
	for tag := range repo.tags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"name": repoName,
		"tags": tags,
	})
}

// serveManifest handles GET/HEAD /v2/{name}/manifests/{reference}
func serveManifest(w http.ResponseWriter, r *http.Request, store *contentStore, repoName, reference string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	repo, ok := store.repos[repoName]
	if !ok {
		writeOCIError(w, http.StatusNotFound, "NAME_UNKNOWN", fmt.Sprintf("repository %q not found", repoName))
		return
	}

	art, ok := repo.resolve(reference)
	if !ok {
		writeOCIError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", fmt.Sprintf("manifest %q not found", reference))
		return
	}

	w.Header().Set("Content-Type", ociManifestMediaType)
	w.Header().Set("Docker-Content-Digest", art.manifestDigest)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(art.manifestBytes)))
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(art.manifestBytes) //nolint:gosec // G705: internal test mock data, not user-tainted
}

// serveBlob handles GET /v2/{name}/blobs/{digest}
func serveBlob(w http.ResponseWriter, store *contentStore, repoName, digest string) {
	repo, ok := store.repos[repoName]
	if !ok {
		writeOCIError(w, http.StatusNotFound, "NAME_UNKNOWN", fmt.Sprintf("repository %q not found", repoName))
		return
	}

	blob, ok := repo.blobs[digest]
	if !ok {
		writeOCIError(w, http.StatusNotFound, "BLOB_UNKNOWN", fmt.Sprintf("blob %q not found", digest))
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(blob.data)))
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob.data)
}

func writeOCIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"errors": []map[string]string{
			{"code": code, "message": message},
		},
	})
}

// --- Content Store ---

type contentStore struct {
	repos map[string]*repository
}

type repository struct {
	tags  map[string]*artifact
	blobs map[string]*blob
}

type artifact struct {
	manifestBytes  []byte
	manifestDigest string
}

type blob struct {
	data      []byte
	mediaType string
}

type ociDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type ociManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	ArtifactType  string            `json:"artifactType,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	Config        ociDescriptor     `json:"config"`
	Layers        []ociDescriptor   `json:"layers"`
}

func (r *repository) resolve(reference string) (*artifact, bool) {
	art, ok := r.tags[reference]
	if ok {
		return art, true
	}
	for _, a := range r.tags {
		if a.manifestDigest == reference {
			return a, true
		}
	}
	return nil, false
}

func newContentStore() *contentStore {
	return &contentStore{
		repos: make(map[string]*repository),
	}
}

func (s *contentStore) listRepositories() []string {
	repos := make([]string, 0, len(s.repos))
	for name := range s.repos {
		repos = append(repos, name)
	}
	sort.Strings(repos)
	return repos
}

type layerDef struct {
	mediaType string
	data      []byte
}

// addArtifact creates a repository with OCI manifest, layers, and tags.
func (s *contentStore) addArtifact(repoName string, tags []string, layers []layerDef) {
	repo := &repository{
		tags:  make(map[string]*artifact),
		blobs: make(map[string]*blob),
	}

	emptyConfig := []byte("{}")
	emptyConfigDigest := computeDigest(emptyConfig)
	repo.blobs[emptyConfigDigest] = &blob{data: emptyConfig, mediaType: ociEmptyConfigType}

	layerDescs := make([]ociDescriptor, 0, len(layers))
	for _, l := range layers {
		digest := computeDigest(l.data)
		repo.blobs[digest] = &blob{data: l.data, mediaType: l.mediaType}
		layerDescs = append(layerDescs, ociDescriptor{
			MediaType: l.mediaType,
			Digest:    digest,
			Size:      int64(len(l.data)),
		})
	}

	manifest := ociManifest{
		SchemaVersion: 2,
		MediaType:     ociManifestMediaType,
		Config: ociDescriptor{
			MediaType: ociEmptyConfigType,
			Digest:    emptyConfigDigest,
			Size:      int64(len(emptyConfig)),
		},
		Layers: layerDescs,
	}

	manifestBytes, _ := json.Marshal(manifest)
	manifestDigest := computeDigest(manifestBytes)

	art := &artifact{
		manifestBytes:  manifestBytes,
		manifestDigest: manifestDigest,
	}

	for _, tag := range tags {
		repo.tags[tag] = art
	}

	s.repos[repoName] = repo
}

// complypackDef defines a ComplyPack artifact to seed into the registry.
type complypackDef struct {
	evaluatorID string
	version     string
	content     []byte // opaque content (tar.gz bytes)
}

// addComplypackArtifact creates a ComplyPack OCI artifact with config and content layers.
// The manifest uses artifactType to identify it as a ComplyPack and stores the
// complypack config blob (evaluator-id + version) as the manifest config descriptor,
// matching the real complypack push layout.
func (s *contentStore) addComplypackArtifact(repoName string, tags []string, def complypackDef) {
	repo := &repository{
		tags:  make(map[string]*artifact),
		blobs: make(map[string]*blob),
	}

	// Config blob — JSON with evaluator-id and version.
	configJSON, _ := json.Marshal(map[string]string{
		"evaluator-id": def.evaluatorID,
		"version":      def.version,
	})
	configDigest := computeDigest(configJSON)
	repo.blobs[configDigest] = &blob{data: configJSON, mediaType: complypackConfigType}

	// Content blob — opaque tar.gz payload.
	contentDigest := computeDigest(def.content)
	repo.blobs[contentDigest] = &blob{data: def.content, mediaType: complypackContentType}

	manifest := ociManifest{
		SchemaVersion: 2,
		MediaType:     ociManifestMediaType,
		ArtifactType:  complypackArtifactType,
		Annotations: map[string]string{
			"complypack.evaluator-id": def.evaluatorID,
		},
		Config: ociDescriptor{
			MediaType: complypackConfigType,
			Digest:    configDigest,
			Size:      int64(len(configJSON)),
		},
		Layers: []ociDescriptor{
			{
				MediaType: complypackContentType,
				Digest:    contentDigest,
				Size:      int64(len(def.content)),
			},
		},
	}

	manifestBytes, _ := json.Marshal(manifest)
	manifestDigest := computeDigest(manifestBytes)

	art := &artifact{
		manifestBytes:  manifestBytes,
		manifestDigest: manifestDigest,
	}

	for _, tag := range tags {
		repo.tags[tag] = art
	}

	s.repos[repoName] = repo
}

func computeDigest(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash)
}

// buildDummyTarGz creates a minimal in-memory tar.gz archive containing a single file.
// Used to produce valid complypack content blobs for demo/testing.
func buildDummyTarGz(name string, content []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	_ = tw.WriteHeader(&tar.Header{
		Name: name,
		Size: int64(len(content)),
		Mode: 0o644,
	})
	_, _ = tw.Write(content)

	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}

// --- Seed Data ---

// seedPolicyFromFiles loads a catalog and policy YAML from embedded testdata
// files and registers them as a versioned OCI artifact in the content store.
func (s *contentStore) seedPolicyFromFiles(
	repoPath, catalogFile, policyFile string, tags []string,
) {
	catalog, err := seedData.ReadFile(catalogFile)
	if err != nil {
		log.Fatalf("failed to load catalog seed data from %s: %v", catalogFile, err)
	}
	policy, err := seedData.ReadFile(policyFile)
	if err != nil {
		log.Fatalf("failed to load policy seed data from %s: %v", policyFile, err)
	}
	s.addArtifact(repoPath, tags, []layerDef{
		{mediaType: gemaraCatalogType, data: catalog},
		{mediaType: gemaraPolicyType, data: policy},
	})
}

func (s *contentStore) seedDefaults() {
	s.seedPolicyFromFiles("policies/test-branch-protection",
		"testdata/test-branch-protection-catalog.yaml",
		"testdata/test-branch-protection-policy.yaml",
		[]string{"v1.0.0", "latest"})

	// policies/ampel-branch-protection — AMPEL branch protection controls
	s.seedPolicyFromFiles("policies/ampel-branch-protection",
		"testdata/ampel-branch-protection-catalog.yaml",
		"testdata/ampel-branch-protection-policy.yaml",
		[]string{"v1.0.0", "latest"})

	// complypacks/ampel-bp — ComplyPack artifact for AMPEL branch protection evaluator
	s.addComplypackArtifact("complypacks/ampel-bp", []string{"v1.0.0", "latest"}, complypackDef{
		evaluatorID: "ampel",
		version:     "1.0.0",
		content:     buildDummyTarGz("policy.json", []byte(`{"name":"ampel-branch-protection","version":"1.0.0"}`)),
	})

}
