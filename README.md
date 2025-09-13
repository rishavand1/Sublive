# Sublive
# Sublive is a lightweight command-line tool written in Go for quickly scanning and checking the liveness of subdomains for a given domain. It resolves subdomains, attempts HTTP/HTTPS connections, and reports status codes and IPs. It supports wordlists from files, stdin, or defaults, with options for recursion depth and concurrency.
Features

Fast subdomain enumeration and liveness checking.
Supports custom wordlists via file or piped input.
Verbose mode for progress tracking.
Output filtering for live subdomains only.
Recursion modes for deeper scanning.
Concurrent processing for efficiency.
Outputs results to stdout or a file, with a summary of findings.

**Installation**

Prerequisites

Go (version 1.16 or later) installed on your system. Download from golang.org.

Building from Source

Clone the repository:<br>

**git clone [https://github.com/rishavand1/sublive.git](https://github.com/rishavand1/Sublive)**<br>

**cd sublive**<br>

Build the binary:<br>

**go build -o sublive sublive.go**<br>

(Optional) Move the binary to a directory in your PATH for global access:<br>

**sudo mv sublive /usr/local/bin/** <br>


Downloading Pre-built Binary <br>

If available, download the latest release from the Releases page and extract the binary for your platform.
Getting Started <br>

Basic Usage

Run the tool with a target domain:
**./sublive -u google.com -w /usr/share/wordlists/amass/subdomains-top1mil-5000.txt -o results.txt -x -v**



This will use the default wordlist and settings to scan subdomains like www.example.com, mail.example.com, etc., and output results with status codes.
Running with Input

Pipe a wordlist from stdin:

**cat wordlist.txt | ./sublive -u example.com**

Use a wordlist file:

**./sublive -u example.com -w wordlist.txt**


Output

Results are printed to stdout by default (e.g., www.example.com 200).
Use -o result.txt to save to a file.
A summary is always printed at the end, categorizing results (live, redirects, 404s, etc.).

Flags and Options
Sublive uses command-line flags for configuration. Run ./sublive without arguments to see the usage help.

-u <domain> (required):
Specifies the target root domain (e.g., -u example.com).
Example: ./sublive -u example.com

-v (optional):
Enables verbose mode, showing progress and status for each checked subdomain.
Default: false (no verbose output).
Example: ./sublive -u example.com -v

-t <level> (optional):
Sets the recursion/speed level:

1: Deep and slow (more words, recursion enabled, fewer workers).
2: Medium (balanced, default).
3: Fast (fewer words, higher concurrency, no recursion).
Default: 2.
Example: ./sublive -u example.com -t 1


-o <file> (optional):
Specifies the output file path to save results. If not provided, outputs to stdout.
Example: ./sublive -u example.com -o results.txt

-x (optional):
Outputs only live subdomains (status codes 200-399) with their status. When set, unreachable or error subdomains are excluded from the output.
Default: false (outputs all checked subdomains).
Example: ./sublive -u example.com -x

-w <file> (optional):
Path to a custom wordlist file. If provided, it's used instead of stdin or defaults.
Example: ./sublive -u example.com -w /path/to/wordlist.txt

Examples

Basic scan with defaults:<br>
**./sublive -u example.com -w (wordlist)**

Verbose scan with output to file:
**./sublive -u example.com -v -o results.txt**

Deep recursive scan using stdin wordlist:
**cat large_wordlist.txt | ./sublive -u example.com -t 1 -v**

Fast scan, only live subdomains:
**./sublive -u example.com -w -t 3 -x**

Using a file wordlist and medium speed:
**./sublive -u example.com -w custom_words.txt -t 2**


Wordlist Priority

If -w is provided, use the file.
Else, if data is piped to stdin, use that.
Else, use built-in defaults (expanded based on -t level).

Notes

The tool skips SSL verification for HTTPS checks (insecure mode) to handle self-signed certs.
Recursion in deep mode (-t 1) generates additional subdomains like sub-stage.example.com for live ones.
Performance scales with -t: Higher levels use more CPU threads.
No external dependencies beyond standard Go libraries.
