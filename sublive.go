// sublive - fast CLI subdomain liveness scanner
// Usage: go build -o sublive && ./sublive -u example.com -t 2 -v -x -o result.txt

package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var version = "0.4"

// small default wordlist. Users should supply more via pipe to stdin or file.
var defaultWords = []string{
	"www", "mail", "ftp", "api", "dev", "test", "stage", "admin", "portal", "beta",
	"shop", "cdn", "m", "mobile", "secure", "webmail",
}

type Result struct {
	Subdomain string
	Status    int
	IP        string
}

func worker(ctx context.Context, domain string, jobs <-chan string, results chan<- Result, verbose bool, client *http.Client, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case sub, ok := <-jobs:
			if !ok {
				return
			}

			// Resolve quickly
			ips, _ := net.LookupHost(sub)
			ip := ""
			if len(ips) > 0 {
				ip = ips[0]
			}

			// Try HTTP then HTTPS with per-request timeout
			reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			status := 0
			// HTTP attempt
			httpReq, _ := http.NewRequestWithContext(reqCtx, "GET", "http://"+sub, nil)
			resp, err := client.Do(httpReq)
			if err == nil && resp != nil {
				status = resp.StatusCode
				resp.Body.Close()
			} else {
				// HTTPS fallback
				httpsReq, _ := http.NewRequestWithContext(reqCtx, "GET", "https://"+sub, nil)
				resp2, err2 := client.Do(httpsReq)
				if err2 == nil && resp2 != nil {
					status = resp2.StatusCode
					resp2.Body.Close()
				}
			}
			cancel()

			if verbose {
				fmt.Printf("[+] checked %s -> %d %s\n", sub, status, ip)
			}

			results <- Result{Subdomain: sub, Status: status, IP: ip}
		}
	}
}

func loadWordlistFromStdin() ([]string, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}
	// If data is being piped
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		out := []string{}
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line != "" {
				out = append(out, line)
			}
		}
		return out, s.Err()
	}
	return nil, nil
}

func loadWordlistFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" {
			out = append(out, line)
		}
	}
	return out, s.Err()
}

func uniqStrings(in []string) []string {
	m := make(map[string]struct{})
	out := []string{}
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := m[s]; !ok {
			m[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

func main() {
	// flags
	domain := flag.String("u", "", "target root domain (e.g. example.com)")
	verbose := flag.Bool("v", false, "verbose - show progress and statuses")
	t := flag.Int("t", 2, "recursion / speed: 1=deep+slow, 2=medium, 3=fast")
	outfile := flag.String("o", "", "output file path (optional)")
	sortLive := flag.Bool("x", false, "output only live subdomains (with status code). When set, only live entries are printed to output")
	wordlistPath := flag.String("w", "", "path to a wordlist file (optional). If provided it is used instead of stdin/defaults")
	flag.Parse()

	if *domain == "" {
		fmt.Println("usage: sublive -u example.com [-t 1..3] [-v] [-x] [-o file] [-w wordlist_file]")
		os.Exit(1)
	}

	start := time.Now()
	if *verbose {
		fmt.Printf("sublive v%s - scanning %s\n", version, *domain)
	}

	// determine wordlist source: -w file > stdin > defaults
	var words []string
	if *wordlistPath != "" {
		w, err := loadWordlistFromFile(*wordlistPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open wordlist '%s': %v\n", *wordlistPath, err)
			os.Exit(1)
		}
		words = w
		if *verbose { fmt.Printf("[+] loaded %d words from %s\n", len(words), *wordlistPath) }
	} else if piped, _ := loadWordlistFromStdin(); piped != nil && len(piped) > 0 {
		words = piped
		if *verbose { fmt.Printf("[+] loaded %d words from stdin\n", len(words)) }
	} else {
		switch *t {
		case 1:
			words = append(defaultWords, "app", "gateway", "auth", "accounts", "login", "payments", "images", "static", "docs", "status", "internal", "ops", "graphql", "socket", "router", "db")
		case 2:
			words = append(defaultWords, "app", "auth", "login", "api", "static", "cdn")
		case 3:
			words = defaultWords
		default:
			words = defaultWords
		}
	}

	words = uniqStrings(words)

	// generate initial candidate subdomains
	candidates := make([]string, 0, len(words))
	for _, w := range words {
		candidates = append(candidates, w+"."+*domain)
	}

	deep := (*t == 1)

	// set concurrency
	workers := 30
	switch *t {
	case 1:
		workers = 30
	case 2:
		workers = 80
	case 3:
		workers = runtime.NumCPU() * 40
	}

	if *verbose { fmt.Printf("[+] workers=%d deep=%v candidates=%d\n", workers, deep, len(candidates)) }

	jobs := make(chan string, 10000)
	results := make(chan Result, 10000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: transport}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker(ctx, *domain, jobs, results, *verbose, client, &wg)
	}

	// producer: feed initial candidates
	go func() {
		for _, c := range candidates {
			jobs <- c
		}
		// Non-deep mode: no more jobs will be added, so close now
		if !deep {
			close(jobs)
		}
	}()

	// collector: read results and optionally add recursive permutations
	found := make(map[string]Result)
	var mu sync.Mutex

	go func() {
		for r := range results {
			mu.Lock()
			if _, ok := found[r.Subdomain]; !ok {
				found[r.Subdomain] = r
			}
			mu.Unlock()

			// if deep and response non-zero, generate a few permutations and enqueue
			if deep && r.Status != 0 {
				parts := strings.Split(r.Subdomain, ".")
				if len(parts) >= 3 {
					sub := parts[0]
					c1 := sub + "-stage." + *domain
					c2 := sub + "-dev." + *domain
					c3 := "api." + sub + "." + *domain
					mu.Lock()
					if _, ok := found[c1]; !ok {
						jobs <- c1
					}
					if _, ok := found[c2]; !ok {
						jobs <- c2
					}
					if _, ok := found[c3]; !ok {
						jobs <- c3
					}
					mu.Unlock()
				}
			}
		}
	}()

	// If deep mode, allow recursion for a limited time then close jobs
	if deep {
		time.Sleep(6 * time.Second)
		close(jobs)
	}

	// wait workers then close results
	wg.Wait()
	close(results)

	// small pause to ensure collector processed remaining
	time.Sleep(200 * time.Millisecond)

	// collect found results
	mu.Lock()
	subs := make([]Result, 0, len(found))
	for _, r := range found {
		subs = append(subs, r)
	}
	mu.Unlock()

	// classify
	counts := map[string]int{"live": 0, "404": 0, "301": 0, "other": 0, "unreachable": 0}
	for _, r := range subs {
		if r.Status == 0 {
			counts["unreachable"]++
		} else if r.Status == 404 {
			counts["404"]++
		} else if r.Status == 301 || r.Status == 302 {
			counts["301"]++
		} else if r.Status >= 200 && r.Status < 400 {
			counts["live"]++
		} else {
			counts["other"]++
		}
	}

	// prepare output lines
	lines := make([]string, 0, len(subs))
	for _, r := range subs {
		lines = append(lines, fmt.Sprintf("%s %d", r.Subdomain, r.Status))
	}
	sort.Strings(lines)

	outLines := []string{}
	if *sortLive {
		for _, r := range subs {
			if r.Status >= 200 && r.Status < 400 {
				outLines = append(outLines, fmt.Sprintf("%s %d", r.Subdomain, r.Status))
			}
		}
	} else {
		outLines = lines
	}

	// write output
	if *outfile != "" {
		f, err := os.Create(*outfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		for _, l := range outLines {
			f.WriteString(l + "\n")
		}
		if *verbose { fmt.Printf("[+] wrote %d lines to %s\n", len(outLines), *outfile) }
	} else {
		for _, l := range outLines {
			fmt.Println(l)
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("\nSummary for %s (t=%d) in %s:\n", *domain, *t, elapsed.Round(time.Millisecond))
	fmt.Printf("  live (2xx): %d\n", counts["live"])
	fmt.Printf("  redirects (301/302): %d\n", counts["301"])
	fmt.Printf("  404: %d\n", counts["404"])
	fmt.Printf("  other: %d\n", counts["other"])
	fmt.Printf("  unreachable: %d\n", counts["unreachable"])

	os.Exit(0)
}
