package store

import (
	"testing"
	"path/filepath"

	"jodata/internal/dataset"
	"jodata/internal/ingest"
)

func TestFileStorePersistsDatasets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jodata.json")
	first, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}})
	first.SaveDataset(ds)

	second, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := second.Dataset("dataset_1")
	if !ok {
		t.Fatal("expected persisted dataset")
	}
	if got.Name != "sales" {
		t.Fatalf("dataset name = %s", got.Name)
	}
}

func TestFileStorePersistsProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jodata.json")
	first, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	first.SaveProfile(ingest.ParserProfile{CustomerID: "acme", HeaderRow: 3})

	second, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := second.Profile("acme")
	if !ok {
		t.Fatal("expected persisted profile")
	}
	if profile.HeaderRow != 3 {
		t.Fatalf("header row = %d", profile.HeaderRow)
	}
}

func TestExportAndImportBundleRoundTrip(t *testing.T) {
	store := NewMemoryStore()
	store.SaveDataset(dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}}))
	imported := store.ExportBundle()
	if imported.Data.Next == 0 {
		t.Fatal("expected next to be set")
	}

	target := NewMemoryStore()
	target.ImportBundle(imported)
	ds, ok := target.Dataset("dataset_1")
	if !ok {
		t.Fatal("expected dataset to be imported")
	}
	if ds.Name != "sales" {
		t.Fatalf("dataset name = %s", ds.Name)
	}
}
