package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/liamg/scout/pkg/scan"
	"github.com/liamg/scout/pkg/wordlist"
	"github.com/liamg/tml"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var statusCodes []string
var filename string
var extensions = scan.DefaultURLOptions.Extensions

var urlCmd = &cobra.Command{
	Use:   "url [url]",
	Short: "Discover URLs on a given web server.",
	Long:  "Scout will discover URLs relative to the provided one.",
	Run: func(cmd *cobra.Command, args []string) {

		log.SetOutput(ioutil.Discard)

		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		}

		if noColours {
			tml.DisableFormatting()
		}

		if len(args) == 0 {
			tml.Println("<bold><red>Error:</red></bold> You must specify a target URL.")
			os.Exit(1)
		}

		parsedURL, err := url.ParseRequestURI(args[0])
		if err != nil {
			tml.Printf("<bold><red>Error:</red></bold> Invalid URL: %s\n", err)
			os.Exit(1)
		}

		resultChan := make(chan scan.URLResult)
		busyChan := make(chan string, 0x400)

		var intStatusCodes []int

		for _, code := range statusCodes {
			i, err := strconv.Atoi(code)
			if err != nil {
				tml.Printf("<bold><red>Error:</red></bold> Invalid status code entered: %s.\n", code)
				os.Exit(1)
			}
			intStatusCodes = append(intStatusCodes, i)
		}

		options := &scan.URLOptions{
			PositiveStatusCodes: intStatusCodes,
			TargetURL:           *parsedURL,
			ResultChan:          resultChan,
			BusyChan:            busyChan,
			Parallelism:         parallelism,
			Extensions:          extensions,
			Filename:            filename,
			SkipSSLVerification: skipSSLVerification,
		}
		if wordlistPath != "" {
			options.Wordlist, err = wordlist.FromFile(wordlistPath)
			if err != nil {
				tml.Printf("<bold><red>Error:</red></bold> %s\n", err)
				os.Exit(1)
			}
		}
		options.Inherit()

		tml.Printf(
			`
<blue>[</blue><yellow>+</yellow><blue>] Target URL</blue><yellow>      %s
<blue>[</blue><yellow>+</yellow><blue>] Routines</blue><yellow>        %d 
<blue>[</blue><yellow>+</yellow><blue>] Extensions</blue><yellow>      %s 
<blue>[</blue><yellow>+</yellow><blue>] Positive Codes</blue><yellow>  %s

`,
			options.TargetURL.String(),
			options.Parallelism,
			strings.Join(options.Extensions, ","),
			strings.Join(statusCodes, ","),
		)

		scanner := scan.NewURLScanner(options)

		waitChan := make(chan struct{})

		genericOutputChan := make(chan string)
		importantOutputChan := make(chan string)

		go func() {
			for result := range resultChan {
				importantOutputChan <- tml.Sprintf("<blue>[</blue><yellow>%d</yellow><blue>]</blue> %s\n", result.StatusCode, result.URL.String())
			}
			close(waitChan)
		}()

		go func() {
			defer func() {
				_ = recover()
			}()
			for uri := range busyChan {
				genericOutputChan <- tml.Sprintf("Checking %s...", uri)
			}
		}()

		outChan := make(chan struct{})
		go func() {

			defer close(outChan)

			for {
				select {
				case output := <-importantOutputChan:
					clearLine()
					fmt.Printf(output)
				FLUSH:
					for {
						select {
						case str := <-genericOutputChan:
							if str == "" {
								break FLUSH
							}
						default:
							break FLUSH
						}
					}
				case <-waitChan:
					return
				case output := <-genericOutputChan:
					clearLine()
					fmt.Printf(output)
				}
			}

		}()

		results, err := scanner.Scan()
		if err != nil {
			clearLine()
			tml.Printf("<bold><red>Error:</red></bold> %s\n", err)
			os.Exit(1)
		}
		logrus.Debug("Waiting for output to flush...")
		<-waitChan
		close(genericOutputChan)
		<-outChan

		clearLine()
		tml.Printf("\n<bold><green>Scan complete. %d results found.</green></bold>\n\n", len(results))

	},
}

func clearLine() {
	fmt.Printf("\033[2K\r")
}

func init() {
	urlCmd.Flags().StringVarP(&filename, "filename", "f", filename, "Filename to seek in the directory being searched. Useful when all directories report 404 status.")
	urlCmd.Flags().StringSliceVarP(&statusCodes, "status-codes", "s", statusCodes, "HTTP status codes which indicate a positive find.")
	urlCmd.Flags().StringSliceVarP(&extensions, "extensions", "x", extensions, "File extensions to detect.")

	rootCmd.AddCommand(urlCmd)
}
