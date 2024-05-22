import * as fs from 'fs'
import split from 'binary-split'
import { PassThrough, Transform } from 'stream'

export interface Opts {
  recording: string
  loop: boolean
  speed: number
}

const playback = (speed: number) =>
  new Transform({
    transform(chunk, _, callback) {
      const data = chunk.toString()
      const items = data.split(',')

      const [timeout, msg] =
        items.length === 2 ? [parseInt(items[0]), items[1]] : [20, items[0]]

      const buf = Buffer.from(msg, 'base64')

      setTimeout(() => {
        this.push(buf)
        callback()
      }, timeout / speed)
    },
  })

export function start(opts: Opts) {
  const passthrough = new PassThrough()
  ;(function loop() {
    fs.createReadStream(opts.recording)
      .pipe(split())
      .pipe(playback(opts.speed))
      .on('end', () => {
        if (opts.loop) {
          loop()
        } else {
          passthrough.end()
        }
      })
      .pipe(passthrough, { end: false })
  })()
  return passthrough
}
