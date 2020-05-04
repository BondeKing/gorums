package gengorums

import (
	"fmt"
	"log"
	"os"

	"google.golang.org/protobuf/compiler/protogen"
)

// GenerateDevFiles generates a zorums_{{gorumsType}}_gorums.pb.go file for each Gorums datatype
// and for each call type in the service definition.
func GenerateDevFiles(gen *protogen.Plugin, file *protogen.File) {
	for gorumsType := range gorumsCallTypesInfo {
		generateDevFile(gen, file, gorumsType)
	}
}

func generateDevFile(gen *protogen.Plugin, file *protogen.File, gorumsType string) {
	if len(file.Services) == 0 || !hasGorumsType(file.Services, gorumsType) {
		// there is nothing for this plugin to do
		fmt.Fprintf(os.Stderr, "ignoring %s\n", gorumsType)
		return
	}
	if len(file.Services) > 1 {
		// To build multiple services, make separate proto files and
		// run the plugin separately for each proto file.
		// These cannot share the same Go package.
		log.Fatalln("Gorums does not support multiple services in the same proto file.")
	}

	// generate dev file for given gorumsType
	filename := file.GeneratedFilenamePrefix + "_" + gorumsType + "_gorums.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	g.P("// Code generated by protoc-gen-gorums. DO NOT EDIT.")
	g.P()
	g.P("package ", file.GoPackageName)
	g.P()
	genGorumsType(g, file.Services, gorumsType)
}
