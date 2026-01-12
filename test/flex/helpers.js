const { wait } = require("../utils");

async function waitForEndpoint(url, maxAttempts = 5, delay = 100) {
  let lastError = null;
  for (let i = 0; i < maxAttempts; i++) {
    try {
      // note: not connecting to websocket to avoid messing with test state by having a client
      const res = await fetch(url);
      return;
    } catch (e) {
      lastError = e;
      await wait(delay);
    }
  }
  throw new Error(`Endpoint ${url} not available after ${maxAttempts} attempts, last error: ${e}`);
}

/**
 * Generate a single synthetic Flex serial data frame.
 *
 * Protocol format:
 * - Header: 'N' + '\n' + length_msb + length_lsb (big-endian uint16)
 * - Body: 'P' + '\n' + samples...
 * - Each sample (8-bit):  row (1 byte) + col (1 byte) + pressure (1 byte)
 * - Each sample (12-bit): row (1 byte) + col (1 byte) + pressure (2 bytes big-endian)
 *
 * Each frame contains two points:
 *   Point 1: (n, 1, n*2+1) - row=n, col=1, pressure=n*2+1
 *   Point 2: (1, n, n*3+1) - row=1, col=n, pressure=n*3+1
 *
 * @param {number} n - Frame index (0-23)
 * @param {number} [bitDepth=12] - Bit depth: 8 for v4, 12 for v5
 * @returns {Buffer} - The serial data for one frame
 */
function generateFlexSerialFrame(n, bitDepth = 12) {
  const numSamples = 2;

  // Header: 'N' + '\n' + length (2 bytes big-endian)
  const header = Buffer.from("N\n");
  const length = Buffer.alloc(2);
  length.writeUInt16BE(numSamples);

  const bodyStart = Buffer.from("P\n");

  const bytesPerSample = bitDepth === 8 ? 3 : 4;

  // Sample 1: (n, 1, n*2+1)
  const sample1 = Buffer.alloc(bytesPerSample);
  sample1[0] = n;               // row
  sample1[1] = 1;               // col
  const pressure1 = n * 2 + 1;
  if (bitDepth === 8) {
    sample1[2] = pressure1 & 0xff;
  } else {
    sample1.writeUInt16BE(pressure1, 2);
  }

  // Sample 2: (1, n, n*3+1)
  const sample2 = Buffer.alloc(bytesPerSample);
  sample2[0] = 1;               // row
  sample2[1] = n;               // col
  const pressure2 = n * 3 + 1;
  if (bitDepth === 8) {
    sample2[2] = pressure2 & 0xff;
  } else {
    sample2.writeUInt16BE(pressure2, 2);
  }

  return Buffer.concat([header, length, bodyStart, sample1, sample2]);
}

/**
 * Generate a single synthetic Sensitronics serial data frame.
 *
 * Protocol format (TLV):
 * - 0xFF: frame start marker (1 byte)
 * - MESSAGE_TYPE: uint8 (1 byte)
 * - MESSAGE_LENGTH: uint16 little-endian (2 bytes)
 * - MESSAGE_VALUE: variable-length data (MESSAGE_LENGTH bytes)
 *
 * @param {number} messageType - The message type (0-255)
 * @param {Buffer} data - The message payload
 * @returns {Buffer} - The complete frame
 */
function generateSensitronicsFrame(messageType, data) {
  const header = Buffer.alloc(4);
  header[0] = 0xff; // start marker
  header[1] = messageType & 0xff; // message type
  header.writeUInt16LE(data.length, 2); // message length

  return Buffer.concat([header, data]);
}

/**
 * Generate a Sensitronics frame with random message type and random data.
 *
 * @param {number} [maxDataLength=100] - Maximum length of random data
 * @returns {Buffer} - The complete frame
 */
function generateRandomSensitronicsFrame(maxDataLength = 100) {
  const messageType = Math.floor(Math.random() * 256);
  const dataLength = Math.floor(Math.random() * maxDataLength);
  const data = Buffer.alloc(dataLength);
  for (let i = 0; i < dataLength; i++) {
    data[i] = Math.floor(Math.random() * 256);
  }

  return generateSensitronicsFrame(messageType, data);
}

/**
 * Split a buffer into random-sized chunks.
 * Useful for simulating fragmented serial transmission.
 *
 * @param {Buffer} buffer - The buffer to split
 * @param {number} [minChunkSize=1] - Minimum chunk size
 * @param {number} [maxChunkSize=10] - Maximum chunk size
 * @returns {Buffer[]} - Array of buffer chunks
 */
function splitBufferRandomly(buffer, minChunkSize = 1, maxChunkSize = 10) {
  const chunks = [];
  let offset = 0;

  while (offset < buffer.length) {
    const remaining = buffer.length - offset;
    const chunkSize = Math.min(
      remaining,
      Math.floor(Math.random() * (maxChunkSize - minChunkSize + 1)) + minChunkSize
    );
    chunks.push(buffer.subarray(offset, offset + chunkSize));
    offset += chunkSize;
  }

  return chunks;
}

module.exports = {
  waitForEndpoint,
  generateFlexSerialFrame,
  generateSensitronicsFrame,
  generateRandomSensitronicsFrame,
  splitBufferRandomly,
};
