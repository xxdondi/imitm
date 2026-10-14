## IMTM - I'M In The Middle

Local MITM proxy server for any needs.

![ci status](https://github.com/xxdondi/imitm/actions/workflows/main.yml/badge.svg)

### Capture traffic from an iPhone

Requires Go 1.21 or newer. From the repository directory:

```sh
go run . -init-ca
go run .
```

The default mode is capture only: HTTPS is intercepted using your own local CA,
and no traffic modification modules are enabled.

1. Connect the Mac and iPhone to the same Wi-Fi network.
2. On the iPhone, open Safari at `http://<Mac-LAN-IP>:3129/ca.cer`.
3. Install the downloaded profile in Settings → General → VPN & Device Management.
4. Enable full trust in Settings → General → About → Certificate Trust Settings.
5. In Settings → Wi-Fi → your network → Configure Proxy → Manual, set Server to
   the Mac's LAN IP and Port to `3129`, with authentication off.
6. Visit an HTTP page and an HTTPS page in Safari, and watch the terminal.

With the proxy configured, `http://imitm.local/ca.cer` also serves the certificate.
The installable file is `certs/imitm-ca.cer`; share only that public certificate,
never `certs/imitm-ca-key.pem`. The same CA is reused across restarts.

Each run creates a directory under `data/captures/` containing `flows.jsonl`
(request/response URLs, headers, status, and flow IDs) and binary body files.
Bodies are captured as they are forwarded, capped at 1 MiB per body by default;
larger captures contain only the prefix, while the full body reaches its recipient.
Use `-body-limit 0` for metadata only or a larger byte limit for bigger captures.
Captures can contain credentials and personal data and are excluded from Git.
An interrupted transfer can leave a partial body file. This records HTTP traffic,
not raw packets or WebSocket message streams.

```sh
go run . -v                          # additional internal proxy logging
go run . -httpaddr :8080             # another explicit proxy port
go run . -capture-dir data/my-runs    # another capture location
go run . -modules youtube -body-limit 16777216 # YouTube preroll blocking
```

`-httpsaddr :3128` enables the optional redirected TLS listener. Manual Wi-Fi
proxy setup uses port 3129 for both HTTP and HTTPS and needs no PF configuration.
Allow incoming connections if the macOS firewall prompts. Apps that ignore the
Wi-Fi proxy or pin certificates may not be captured. Turn Configure Proxy back
to Off when finished.

### Inspect protobuf captures offline

```sh
go run ./cmd/protoinspect data/captures/<run>/<flow>-response.bin
```

Accepts multiple files. Writes a `.pscope.txt` disassembly and `.analysis.json`
report next to each input, leaving the captured bytes intact. The report checks
top-level wire validity and byte-identical decode/re-encode, and traces ad markers
through inferred enclosing fields with byte offsets. Without the schema, a
length-delimited field could be a message, string, packed numbers, or opaque bytes;
the inferred structure and ad markers are evidence for investigation, not removal
rules. Decoded files inherit the sensitivity of the original captures.

For `application/vnd.yt-ump` responses, use the separate framing inspector:

```sh
go run ./cmd/umpinspect data/captures/<run>/<flow>-response.bin
```

It creates a `.ump/` directory with `parts.json`, extracted complete control
payloads, and protobuf disassemblies where the wire format is valid. Media bytes
remain in the original file. It reports incomplete parts rather than assuming
they are malformed, since protocol continuation and capture truncation can both
produce them. Onesie header type 25 indicates an encrypted InnerTube response:
a valid protobuf envelope does not mean its inner player data is decrypted.
The inspector identifies framing only; it does not decrypt or alter traffic.

### YouTube module

```sh
go run . -modules youtube -body-limit 16777216
```

The YouTube module combines two observed behaviors:

1. Return local HTTP 502 for POST `/initplayback` on `*.googlevideo.com`, which
   caused the tested iOS app to fall back to plaintext `/youtubei/v1/player`.
2. Remove guarded direct fields 7 and 68 from that player response, and from the
   content player at `1 / 2` in `/youtubei/v1/get_watch` external-link responses.

Removal requires content video details, format metadata, and the observed
field-68 ad-slot wrapper containing an ad URL marker. All other player fields,
content URLs and the separate UI envelope retain their original bytes. Enclosing
protobuf lengths and HTTP Content-Length are updated. Ad-free player responses
remain unchanged, including small field-7 structures without an ad slot.

Only plaintext HTTP 200 protobuf player responses on `youtubei.googleapis.com`
are rewritten. Explicitly encoded, malformed, unsupported or over-16-MiB bodies
pass through. GET initialization, `/videoplayback` and unrelated traffic remain
available. Blocking initialization may introduce fallback latency or playback
errors; coverage of midrolls and other app versions is not established.

`INITPLAYBACK502` logs and metadata record blocked requests; `PLAYERADS` logs
and `playerads` metadata record removed fields and the endpoint. Paired
`*-playerads-original.bin` and `*-playerads-modified.bin` files preserve evidence.
Ordinary body captures retain upstream originals. Use `go run .` for unmodified
capture mode.

### Adding modules

The main application owns the proxy listeners, TLS interception, certificate
distribution and the capture recorder in `internal/capture`. Features live in separate Go packages under
`internal/modules/`. Go packages are directories of source files compiled into
the same executable; adding a module does not require a separate `go.mod`.

`internal/modules.Module` has two methods: `Name() string` and
`Register(*goproxy.ProxyHttpServer)`. A module installs its request/response
handlers using goproxy. The YouTube implementation is in
`internal/modules/youtube`, including its protobuf parsing and removal rules.
YouTube unit and proxy integration tests live alongside the module.
Its constructor accepts `modules.Evidence`, which provides `Record` and
`SaveArtifact` methods. This keeps recording decisions and samples independent
of the capture directory layout.

To add a feature, create its package, implement the module interface, and add
its constructor to `availableModules` in `proxy.go`. Enable modules with
`-modules youtube,another`. Modules register in the listed order, after capture
handlers; request bodies and upstream response bodies are therefore recorded
before rewriting. Request handlers should return `(req, nil)` to continue or
a response to stop upstream forwarding. Response handlers should return the
response unchanged when their rule does not apply.

Module selection is validated at startup; unknown or duplicate names fail.
`-modules ""` enables capture only. The existing `-mode adblock` command remains
a shortcut for `-modules youtube`; an explicit `-modules` value takes precedence.
Modules are selected at startup and compiled into the executable.

The failed tag-corruption, preroll-flag, skip-delay and legacy modifiers have
been removed. Historical results remain in
[the investigation record](docs/youtube-ios-investigation.md). The old `adblock`
mode meant tag corruption; it now selects the combined approach described here.
