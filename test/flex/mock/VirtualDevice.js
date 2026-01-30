const VirtualSerialPort = require("./VirtualSerialPort");
const fs = require("fs");
const { promisify } = require("util");
const sleep = promisify(setTimeout);

class VirtualDevice {
  constructor(usbInfo = {}) {
    // Note: on Linux, go.bug.st/serial/enumerator reads directly from sysFS
    // files and treats the data as strings. So to test that our code converts
    // them to uint16 correctly, we have to specify them as strings too.
    //
    // sysFS stores USB descriptors as fixed-length (4 char) hex strings WITHOUT
    // the 0x prefix, so e.g.
    this.idVendor = usbInfo.idVendor || "F0FA";
    this.idProduct = usbInfo.idProduct || "0001";
    this.bcdDevice = usbInfo.bcdDevice || "0001";
    this.serialNumber = usbInfo.serialNumber || "9090909";
    this.manufacturer = usbInfo.manufacturer || "Mockfactory";
    this.product = usbInfo.product || "Mockdevice";

    this.serialPort = new VirtualSerialPort();
    this.registeredId = null;
    this.replayStopRequested = false;
    this.address = null;
  }

  async initialize() {
    this.address = await this.serialPort.open();
  }

  async registerWithDriver(url) {
    if (this.registeredId !== null) {
      throw new Error("Device is already registered");
    }

    if (this.serialPort.getPortPath() === null) {
      throw new Error("Serial port is not created");
    }

    const payload = {
      Name: this.serialPort.getPortPath(),
      VID: this.idVendor,
      PID: this.idProduct,
      BcdDevice: this.bcdDevice,
      SerialNumber: this.serialNumber,
      Manufacturer: this.manufacturer,
      Product: this.product,
    };

    const response = await fetch(`${url}/flex/mock/`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      throw new Error(
        `Failed to register device: ${response.status} ${response.statusText}`,
      );
    }

    const result = await response.json();
    this.registeredId = result.id;

    return this.registeredId;
  }

  async unregisterFromDriver(url) {
    if (this.registeredId === null) {
      throw new Error("Device is not registered");
    }

    const response = await fetch(`${url}/flex/mock/${this.registeredId}`, {
      method: "DELETE",
    });

    if (!response.ok) {
      throw new Error(
        `Failed to unregister device: ${response.status} ${response.statusText}`,
      );
    }

    this.registeredId = null;
  }

  isRegistered() {
    return this.registeredId !== null;
  }

  stopReplay() {
    this.replayStopRequested = true;
  }

  async replayRecording(filePath, loop = true, speedFactor = 1) {
    this.replayStopRequested = false;
    if (!this.serialPort || !this.serialPort.isOpen) {
      throw new Error("Serial port is not open");
    }

    do {
      try {
        const fileContent = fs.readFileSync(filePath, "utf8");
        const lines = fileContent.trim().split("\n");

        for (const line of lines) {
          if (this.replayStopRequested) {
            return;
          }

          if (line.trim() === "") continue;

          const [sleepDurationStr, base64Data] = line.split(",");
          const sleepDuration = parseInt(sleepDurationStr.trim(), 10);

          if (isNaN(sleepDuration) || !base64Data) {
            console.warn(`Skipping invalid line: ${line}`);
            continue;
          }

          // Sleep for specified duration (adjusted by speed factor)
          // Note: sleeping before writing the data to simulate the amount
          // of time it took the device to produce the sample.
          const adjustedSleepDuration = sleepDuration / speedFactor;
          await sleep(adjustedSleepDuration);

          // Convert base64 to binary data
          const binaryData = Buffer.from(base64Data.trim(), "base64");

          // Write data to serial port
          const writeSuccess = this.serialPort.write(binaryData);
          if (!writeSuccess) {
            console.warn(`Failed to write frame data to serial port`);
          }
        }
      } catch (error) {
        throw new Error(`Failed to replay recording: ${error.message}`);
      }
    } while (loop);
  }
}

module.exports = VirtualDevice;
