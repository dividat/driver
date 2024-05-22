import * as Application from './application'
import * as Bootloader from './bootloader'
import * as Profile from './profile'
import * as Replay from '../replay'

// The mock senso has to be reachable through its resolved
// ip address on the local network, since that is what the mDNS discovery
// of the driver will report. Therefore we have to bind to '0.0.0.0'.
const HOST = '0.0.0.0'

// Simulate a delay when switching between Application and DFU modes.
const bootDelay = 5000;

const sleep = (ms: number) =>
  new Promise<void>((resolve) => setTimeout(() => resolve(), ms))

export interface Opts {
  firmwareVersion: Profile.Version
  startInDfu: boolean
}

export async function mock(replayOpts: Replay.Opts, opts: Opts) {
  const application = Application.setup(HOST, replayOpts)
  const bootloader = Bootloader.setup(HOST)

  application.on('EnterDFU', async () => {
    await application.stop()
    await sleep(bootDelay)
    await bootloader.start()
  })

  bootloader.onClose(async () => {
    await sleep(bootDelay)
    application.start()
  })

  if (opts.startInDfu) {
    await bootloader.start()
  } else {
    await application.start()
  }
}
