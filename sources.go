package solgo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	sources_pb "github.com/txpull/protos/dist/go/sources"
	"github.com/txpull/solgo/metadata"
)

var ErrPathFound = errors.New("path found")

// SourceUnit represents a unit of source code in Solidity. It includes the name, path, and content of the source code.
type SourceUnit struct {
	Name    string `yaml:"name" json:"name"`
	Path    string `yaml:"path" json:"path"`
	Content string `yaml:"content" json:"content"`
}

// ToProto converts a SourceUnit to a protocol buffer SourceUnit.
func (s *SourceUnit) ToProto() *sources_pb.SourceUnit {
	return &sources_pb.SourceUnit{
		Name:    s.Name,
		Path:    s.Path,
		Content: s.Content,
	}
}

// Sources represents a collection of SourceUnit. It includes a slice of SourceUnit and the name of the entry source unit.
type Sources struct {
	SourceUnits          []*SourceUnit `yaml:"source_units" json:"source_units"`
	EntrySourceUnitName  string        `yaml:"entry_source_unit" json:"base_source_unit"`
	MaskLocalSourcesPath bool          `yaml:"mask_local_sources_path" json:"mask_local_sources_path"`
	LocalSourcesPath     string        `yaml:"local_sources_path" json:"local_sources_path"`
}

// ToProto converts a Sources to a protocol buffer Sources.
func (s *Sources) ToProto() *sources_pb.Sources {
	var sourceUnits []*sources_pb.SourceUnit
	for _, sourceUnit := range s.SourceUnits {
		sourceUnits = append(sourceUnits, sourceUnit.ToProto())
	}

	return &sources_pb.Sources{
		EntrySourceUnitName:  s.EntrySourceUnitName,
		MaskLocalSourcesPath: s.MaskLocalSourcesPath,
		LocalSourcesPath:     s.LocalSourcesPath,
		SourceUnits:          sourceUnits,
	}
}

// NewSourcesFromMetadata creates a Sources from a metadata package ContractMetadata.
// This is a helper function that ensures easier integration when working with the metadata package.
func NewSourcesFromMetadata(md *metadata.ContractMetadata) *Sources {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	sourcesDir := filepath.Clean(filepath.Join(dir, "sources"))
	sources := &Sources{
		MaskLocalSourcesPath: true,
		LocalSourcesPath:     sourcesDir,
	}

	// First target is the target of the entry source unit...
	for _, name := range md.Settings.CompilationTarget {
		sources.EntrySourceUnitName = name
		break
	}

	// Getting name looks surreal and easy, probably won't work in all cases and is
	// too good to be true.
	for name, source := range md.Sources {
		sources.SourceUnits = append(sources.SourceUnits, &SourceUnit{
			Name:    strings.TrimRight(filepath.Base(name), ".sol"),
			Path:    name,
			Content: source.Content,
		})
	}

	return sources
}

// Prepare validates and prepares the Sources. It checks if each SourceUnit has either a path or content and a name.
// If a SourceUnit has a path but no content, it reads the content from the file at the path.
func (s *Sources) Prepare() error {

	// We should verify that path can be discovered if local sources path is
	// provided.
	if s.LocalSourcesPath != "" {
		if _, err := os.Stat(s.LocalSourcesPath); err != nil {
			return fmt.Errorf("local sources path %s does not exist", s.LocalSourcesPath)
		}
	} else {
		s.LocalSourcesPath = "./sources/"
	}

	for _, sourceUnit := range s.SourceUnits {
		if sourceUnit.Path == "" && sourceUnit.Content == "" {
			return fmt.Errorf("source unit must have either path or content")
		}

		if sourceUnit.Name == "" {
			return fmt.Errorf("source unit must have a name")
		}

		if sourceUnit.Path != "" && sourceUnit.Content == "" {
			content, err := os.ReadFile(sourceUnit.Path)
			if err != nil {
				return err
			}
			sourceUnit.Content = string(content)
		}

		// Extract import statements as perhaps some of them can be found in
		// local sources path and need to be prepended to the sources.
		importUnits, err := s.handleImports(sourceUnit)
		if err != nil {
			return err
		}

		s.SourceUnits = append(s.SourceUnits, importUnits...)
	}

	return nil
}

// GetCombinedSource combines the content of all SourceUnits in the Sources into a single string, separated by two newlines.
func (s *Sources) GetCombinedSource() string {
	var builder strings.Builder
	for i, sourceUnit := range s.SourceUnits {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(sourceUnit.Content)
	}
	return builder.String()
}

// GetSourceUnitByName returns the SourceUnit with the given name from the Sources. If no such SourceUnit exists, it returns nil.
func (s *Sources) GetSourceUnitByName(name string) *SourceUnit {
	for _, sourceUnit := range s.SourceUnits {
		if sourceUnit.Name == name {
			return sourceUnit
		}
	}
	return nil
}

// GetSourceUnitByPath returns the SourceUnit with the given path from the Sources. If no such SourceUnit exists, it returns nil.
func (s *Sources) GetSourceUnitByPath(path string) *SourceUnit {
	for _, sourceUnit := range s.SourceUnits {
		if sourceUnit.Path == path {
			return sourceUnit
		}
	}
	return nil
}

// GetLocalSource attempts to find a local source file that matches the given partial path.
// It searches relative to the provided path and returns a SourceUnit representing the found source.
// If no matching source is found, it returns nil.
//
// The function replaces any instance of "@openzeppelin" in the partial path with the actual path to the openzeppelin-contracts repository.
// It then walks the file tree starting from "./sources/", checking each file against the new path.
//
// If the new path contains "../", it removes this and looks for the file in the parent directory.
// If a match is found, it creates a new SourceUnit with the name and path of the file, and returns it.
//
// If no "../" is present in the new path, it simply creates a new SourceUnit with the name and path.
//
// After a SourceUnit is created, the function checks if the file at the path exists.
// If it does, it reads the file content and assigns it to the SourceUnit's Content field.
// If the file does not exist, it returns an error.
//
// If the walk function encounters an error other than ErrPathFound, it returns the error.
// If the source is still nil after the walk, it returns nil.
func (s *Sources) GetLocalSource(partialPath string, relativeTo string) (*SourceUnit, error) {
	// Replace @openzeppelin with the actual path to the openzeppelin-contracts repository
	partialPath = replaceOpenZeppelin(partialPath)
	relativeTo = replaceOpenZeppelin(relativeTo)
	var source *SourceUnit
	errWalk := filepath.Walk(s.LocalSourcesPath, func(partialWalkPath string, info os.FileInfo, err error) error {
		if info == nil || info.IsDir() {
			return nil
		}

		relativeToDir := filepath.Dir(relativeTo)
		newPath := filepath.Join(relativeToDir, partialPath)

		// If file contains ../, remove it and look for the file in the parent directory
		if strings.Contains(newPath, "../") {
			newPath = strings.TrimSpace(strings.Replace(newPath, "../", "", -1))
			if strings.Contains(partialWalkPath, newPath) {
				sourceName := strings.TrimSuffix(filepath.Base(newPath), ".sol")
				if !s.SourceUnitExists(sourceName) {
					source = &SourceUnit{
						Name: sourceName,
						Path: partialWalkPath,
					}
				}
				return ErrPathFound
			}
		}

		sourceName := strings.TrimSuffix(filepath.Base(newPath), ".sol")
		if !s.SourceUnitExists(sourceName) {
			if strings.Contains(partialWalkPath, newPath) {
				source = &SourceUnit{
					Name: sourceName,
					Path: partialWalkPath,
				}
			}
		}

		return nil
	})

	if errWalk != nil && errWalk != ErrPathFound {
		return nil, errWalk
	}

	if source == nil {
		return nil, nil
	}

	if _, err := os.Stat(source.Path); os.IsNotExist(err) {
		return nil, err
	}

	content, err := os.ReadFile(source.Path)
	if err != nil {
		return nil, err
	}

	source.Content = string(content)
	return source, nil
}

// handleImports extracts import statements from the source unit and adds them to the sources.
func (s *Sources) handleImports(sourceUnit *SourceUnit) ([]*SourceUnit, error) {
	imports := extractImports(sourceUnit.Content)
	var sourceUnits []*SourceUnit

	for _, imp := range imports {
		baseName := filepath.Base(imp)

		if !s.SourceUnitExists(baseName) {
			source, err := s.GetLocalSource(imp, sourceUnit.Path)
			if err != nil {
				return nil, err
			}

			// Source may not be found and no errors and that's ok, however, we don't want to append
			// nil source to the sources.
			if source == nil {
				continue
			}

			if !s.SourceUnitExistsIn(source.Name, sourceUnits) {
				sourceUnits = append(sourceUnits, source)
			}

			subUnits, err := s.handleImports(source)
			if err != nil {
				return nil, err
			}

			for _, subUnit := range subUnits {
				if !s.SourceUnitExistsIn(subUnit.Name, sourceUnits) {
					sourceUnits = append(sourceUnits, subUnit)
				}
			}
		}
	}

	return sourceUnits, nil
}

func (s *Sources) SourceUnitExists(name string) bool {
	return s.SourceUnitExistsIn(name, s.SourceUnits)
}

func (s *Sources) SourceUnitExistsIn(name string, units []*SourceUnit) bool {
	for _, sourceUnit := range units {
		if sourceUnit.Name == name {
			return true
		}
	}
	return false
}

func extractImports(content string) []string {
	re := regexp.MustCompile(`import "(.*?)";`)
	matches := re.FindAllStringSubmatch(content, -1)

	imports := make([]string, len(matches))
	for i, match := range matches {
		imports[i] = match[1]
	}

	return imports
}

func replaceOpenZeppelin(path string) string {
	return strings.Replace(path, "@openzeppelin", filepath.Join("./sources/", "openzeppelin"), 1)
}
