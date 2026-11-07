# pion-dtls-early-appdata-poc

Minimal repro for application data being dropped when a client writes immediately
after completing a Pion DTLS handshake.

## Setup

- Go 1.24 or later
- `v3/`: Pion DTLS v3.1.15
- `v4/`: Pion DTLS v4.0.0-rc.7

The test delays the server after it sends its last handshake flight. The client
then sends three records as soon as its handshake completes.

The delay follows the epoch 1 handshake record (the encrypted Finished). The v4
test pins DTLS 1.2 because later releases default to 1.3, which uses a different
record layout.

## Reproduce

```bash
cd v3
go test -run TestEarlyApplicationData -count=1 -v ./...
```

To compare releases from the `dtls-early-appdata` directory:

```bash
./run.sh v3.1.15 v4.0.0-rc.7 v4.0.1
```

The script uses temporary module files and runs each target three times by
default; set `COUNT` to change the number of runs.

## Expected

The server reads all three records: `one`, `two`, and `three`.

## Actual

With v3.1.15 and v4.0.0-rc.7, the server reads only the first record:

```text
read 2 of 3 failed: dtls timeout: read/write timeout: context deadline exceeded; server read ["one"], client sent ["one" "two" "three"]
```

The same test passes with v4.0.1.
