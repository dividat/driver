# Recording and replaying Flex data

This document provides additional details on how you can record and replay Flex
data.

## Recording Flex data

There are 3 ways to record Senso Flex data:

1. Reading data directly from the serial device, e.g.

        socat stdio /dev/ttyACM0 > recording.dat

   However, this means you cannot run the driver and thus cannot interact with
   the device. In particular, any setup commands would have to be either
   executed manually or prior to starting the recording.

   Not recommended and not supported for replay. timestamping+base64 left as an
   exercise.

2. Recording serial data by spying on driver's reads using `strace`:

        ./tools/record-flex-serial -o recording.serial.dat

   By default, the script will attach to `pidof dividat-driver` and spy on reads
   from `/dev/ttyACM0`, but you can override it with `-p` and `-d` flags. See
   `./tools/record-flex-serial --help` for details.

   This records serial data that can be then be used to do end-to-end replays
   (that involve the driver's processing).

   This recording method only works on Linux. You can still replay these
   recordings on macOS.

   To replay the data, use

        node tools/replay-flex -d <device-type> recording.serial.dat

   By convention, such recordings are suffixed with .<devicetype>.serial.dat

3. Recording the WebSocket binary stream from `/flex`:

   This records the processed output as produced by the driver, using the same
   mechanism as for the Senso.

        make record-flex > recording.dat

   This data can be replayed using a special "passthru" mock device that
   pretends to be a different device to the client:

        node tools/replay-flex -d passthru-<type> recording.ws.dat

   This method can be useful if:
   - You need to capture the exact output of the driver instead of the device
     (e.g. for diff'ing)
   - You cannot use `strace` for recording (e.g. on macOS)

   Note: for `v6` (Sensitronics) devices, the driver outputs identical bytes to the
   serial data read, just chunked/framed (i.e. `concat(serial out) ==
   concat(WS binary stream)`). This means you can record the websocket
   stream, but still replay as if it was recorded directly from the serial
   output (using `-d v6`).

   By convention, such recordings are suffixed with .<devicetype>.ws.dat

## Replaying Flex recordings

The Senso Flex replayer (`npm run replay-flex`) supports the same parameters as
the Senso replayer and also allows to fake device metadata.

Flex replay works by creating a mock serial device (using
`test/flex/mock/VirtualDevice.js`) and registering it in the Driver.

Driver must be running in test mode to allow mock device registration:

    ./bin/dividat-driver -test-mode

You can then replay a recording using:

    node tools/replay-flex -d <device> recording.dat

If you are using a serial data recording (i.e. recorded using
`tools/record-flex-serial`), the Driver will parse it as if it was reading the
serial data from a real device. This can be used to e.g. do regression testing
of the parsing logic in the Driver.

If you are using a WebSocket stream recording (i.e. recorded using `make
record-flex`), you must also specify `--passthru` mode. This will instruct the
Driver to bypass device-specific serial data parsing and instead to simply
forward the serial data over the WebSocket verbatim.

Note: if you are using `passthru` and the recorded data is not chunked into
frames (e.g. it is a recording of raw serial data), the client will receive
incomplete/split frames as separate WebSocket messages. This will not work with
Play, since it expects to receive complete frames. TL;DR: do not use
`passthru` with recordings obtained using `tools/record-flex-serial`.

See
[src/dividat-driver/flex/devices/passthru/main.go](src/dividat-driver/flex/devices/passthru/main.go)
for the implementation details of `passthru`.

CLI help is available with:
    
    node tools/replay-flex --help
