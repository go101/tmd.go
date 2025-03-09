// This is an example program which uses the tmd.go lib.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go101.org/nstd"
	"go101.org/tmd.go"
)

// ToDo: lib version and the command version might be different.
func printUsage(version []byte) {

	fmt.Printf(`TapirMD toolset v%s

Usages:
	%[2]v gen [--support-html-custom-blocks] foo.tmd bar.tmd
	%[2]v fmt foo.tmd bar.tmd
`,
		version,
		filepath.Base(os.Args[0]),
	)
}

func main() {
	tmdLib, err := tmd.NewLib()
	if err != nil {
		log.Fatal(err)
	}
	defer tmdLib.Destroy()

	libVersion, err := tmdLib.Version()
	if err != nil {
		log.Fatal(err)
	}

	flag.Parse()

	args := flag.Args()
	switch len(args) {
	case 0:
		printUsage(libVersion)
		return
	default:
		switch sub := args[0]; sub {
		default:
			nstd.Printfln("Unkown sub-command: %s", sub)
			printUsage(libVersion)
		case "gen":
			if len(args) == 1 {
				printUsage(libVersion)
				return
			}
			generateHTML(tmdLib, args[1:])
		case "fmt":
			if len(args) == 1 {
				printUsage(libVersion)
				return
			}
			formatTMD(tmdLib, args[1:])
		}
	}
}

func generateHTML(tmdLib *tmd.Lib, args []string) {
	const tmdExt = ".tmd"
	const htmlExt = ".html"

	const keySupportHtmlCustomBlocks = "support-html-custom-blocks"

	var options = tmd.HtmlGenOptions{
		RenderRoot: true,
	}

	for i, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			args = args[i:]
			break
		}

		arg = arg[2:]
		argKeyValue := ""
		for k, c := range arg {
			if c == '-' {
				continue
			}
			argKeyValue = arg[k:]
			break
		}
		if len(argKeyValue) == 0 {
			args = args[i+1:]
			break
		}
		if argKeyValue == keySupportHtmlCustomBlocks {
			options.EnabledCustomApps = "html"
		}
	}

	for _, arg := range args {
		var tmdFilePath = arg
		tmdData, err := os.ReadFile(tmdFilePath)
		if err != nil {
			log.Printf("read TMD file [%s] error: %s", tmdFilePath, err)
			continue
		}
		htmlData, err := tmdLib.GenerateHtmlFromTmd(tmdData, options)
		if err != nil {
			log.Printf("geneate HTML file [%s] error: %s", tmdFilePath, err)
			continue
		}

		var htmlFilepath string
		if strings.HasSuffix(strings.ToLower(tmdFilePath), tmdExt) {
			htmlFilepath = tmdFilePath[0:len(tmdFilePath)-len(tmdExt)] + htmlExt
		} else {
			htmlFilepath = tmdFilePath + htmlExt
		}
		err = os.WriteFile(htmlFilepath, htmlData, 0644)
		if err != nil {
			log.Printf("write HTML file [%s] error: %s", htmlFilepath, err)
			continue
		}

		fmt.Printf(`%s (%d bytes)
-> %s (%d bytes)
`, tmdFilePath, len(tmdData), htmlFilepath, len(htmlData))
	}
}

func formatTMD(tmdLib *tmd.Lib, args []string) {
	for _, arg := range args {
		var tmdFilePath = arg
		fileInfo, err := os.Stat(tmdFilePath)
		if err != nil {
			log.Printf("stat TMD file [%s] error: %s", tmdFilePath, err)
			continue
		}
		tmdData, err := os.ReadFile(tmdFilePath)
		if err != nil {
			log.Printf("read TMD file [%s] error: %s", tmdFilePath, err)
			continue
		}
		// fileInfo.Size() == len(tmdData)
		formatData, err := tmdLib.FormatTmd(tmdData)
		if err != nil {
			log.Printf("format TMD file [%s] error: %s", tmdFilePath, err)
			continue
		}

		if formatData != nil {
			err = os.WriteFile(tmdFilePath, formatData, fileInfo.Mode())
			if err != nil {
				log.Printf("write TMD file [%s] error: %s", tmdFilePath, err)
				continue
			}

			fmt.Printf("%s\n", tmdFilePath)
		}
	}
}
