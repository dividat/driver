const { spawn } = require("child_process");
const { EventEmitter } = require("events");
const fs = require("fs");
const path = require("path");
const os = require("os");
// enable debug logs by passing an setting env DEBUG=VirtualSerialPort
const debug = require("debug")("VirtualSerialPort")

class VirtualSerialPort extends EventEmitter {
  constructor() {
    super();
    this.socatProcess = null;
    this.ttyFd = null;
    this.readStream = null;
    this.writeStream = null;
    this.ttyPath = null;
    this.subsidiaryPath = null;
    this.isOpen = false;
  }

  /**
   * @async
   * @returns {Promise<string>} subsidiary TTY path
   */
  async open() {
    debug("Opening virtual serial port");
    return new Promise((resolve, reject) => {
      const tmpDir = os.tmpdir();
      const timestamp = Date.now();
      this.subsidiaryPath = path.join(tmpDir, `vtty_subsidiary_${timestamp}`);
      debug("Creating subsidiary path:", this.subsidiaryPath);

      this.socatProcess = spawn(
        "socat",
        ["-", `pty,raw,echo=0,link=${this.subsidiaryPath}`],
        {
          stdio: ["pipe", "pipe", "pipe"],
        },
      );

      this.socatProcess.on("error", (error) => {
        console.error("socat process error:", error.message);
        this.emit(
          "error",
          new Error(`Failed to spawn socat: ${error.message}`),
        );
        reject(error);
      });

      const waitForSubsidiary = () => {
        try {
          fs.accessSync(this.subsidiaryPath, fs.constants.F_OK);
          debug(
            "Subsidiary path accessible, setting up TTY",
          );
          this.setupTTY();
          this.isOpen = true;
          debug("Port opened successfully");
          this.emit("open");
          resolve(this.subsidiaryPath);
        } catch (e) {
          setTimeout(waitForSubsidiary, 10);
        }
      };

      setTimeout(waitForSubsidiary, 10);

      this.socatProcess.on("exit", (code, signal) => {
        debug(
          "socat process exited with code:",
          code,
          "signal:",
          signal,
        );
        this.isOpen = false;
        this.emit("close");
        if (code !== 0 && code !== null && signal !== "SIGTERM") {
          console.error(
            "socat exited with error code:",
            code,
          );
          this.emit(
            "error",
            new Error(`socat process exited with code ${code}`),
          );
        }
      });
    });
  }

  setupTTY() {
    debug("Setting up TTY streams");
    try {
      this.writeStream = this.socatProcess.stdin;
      this.readStream = this.socatProcess.stdout;

      this.readStream.on("data", (data) => {
        debug("Received data:", data.length, "bytes");
        this.emit("data", data);
      });

      this.readStream.on("error", (error) => {
        console.error("Read stream error:", error.message);
        this.emit("error", error);
      });

      this.writeStream.on("error", (error) => {
        console.error("Write stream error:", error.message);
        this.emit("error", error);
      });

      this.readStream.on("end", () => {
        debug("Read stream ended");
        if (this.isOpen) {
          debug(
            "Emitting close event due to read stream end",
          );
          this.emit("close");
          this.isOpen = false;
        }
      });
    } catch (error) {
      console.error("Error setting up TTY:", error.message);
      this.emit("error", error);
    }
  }

  /**
   * @param {Buffer|string} data
   * @returns {boolean} success
   */
  write(data) {
    if (!this.isOpen || !this.writeStream) {
      console.error(
        "Write failed: TTY is not open (isOpen:",
        this.isOpen,
        "writeStream:",
        !!this.writeStream,
        ")",
      );
      this.emit("error", new Error("TTY is not open"));
      return false;
    }

    try {
      const buffer = Buffer.isBuffer(data) ? data : Buffer.from(data);
      debug("Writing data:", buffer.length, "bytes");
      return this.writeStream.write(buffer);
    } catch (error) {
      console.error("Write error:", error.message);
      this.emit("error", error);
      return false;
    }
  }

  close() {
    if (!this.isOpen) {
      debug("Close called but port already closed");
      return;
    }

    debug("Closing virtual serial port");
    this.isOpen = false;

    if (this.writeStream && !this.writeStream.destroyed) {
      debug("Ending write stream");
      this.writeStream.end();
    }

    this.readStream = null;
    this.writeStream = null;

    if (this.socatProcess && !this.socatProcess.killed) {
      debug("Killing socat process");
      // Remove all listeners to prevent errors during cleanup
      this.socatProcess.removeAllListeners();
      this.socatProcess.kill("SIGTERM");

      setTimeout(() => {
        if (this.socatProcess && !this.socatProcess.killed) {
          debug("Force killing socat process");
          this.socatProcess.kill("SIGKILL");
        }
      }, 1000);
    }

    setTimeout(() => {
      this.cleanupPaths();
    }, 500);
  }

  cleanupPaths() {
    try {
      if (this.subsidiaryPath && fs.existsSync(this.subsidiaryPath)) {
        fs.unlinkSync(this.subsidiaryPath);
      }
    } catch (error) {
      // Ignore cleanup errors
    }
  }

  /**
   * @returns {string|null} subsidiary TTY path
   */
  getPortPath() {
    return this.subsidiaryPath;
  }
}

module.exports = VirtualSerialPort;
