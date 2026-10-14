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
and the experimental YouTube response modifier is disabled.

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
go run . -mode youtube               # experimental, modifies responses
```

`-httpsaddr :3128` enables the optional redirected TLS listener. Manual Wi-Fi
proxy setup uses port 3129 for both HTTP and HTTPS and needs no PF configuration.
Allow incoming connections if the macOS firewall prompts. Apps that ignore the
Wi-Fi proxy or pin certificates may not be captured. Turn Configure Proxy back
to Off when finished.
