import * as path from 'path'
import * as Replay from './lib/replay'
import * as Senso from './lib/senso'
import * as Flex from './lib/flex'
import * as commander from 'commander'
import * as Profile from './lib/senso/profile'

const parseVersion = (value: string): Profile.Version => {
  const match = value.match(/^(\d+)\.(\d+)\.(\d+)$/)
  if (match) {
    const [major, minor, patch] = match
      .slice(1)
      .map((i) => parseInt(i, 10)) as any
    return {
      ...Profile.defaultVersion,
      major,
      minor,
      patch,
    }
  }
  throw new commander.InvalidArgumentError(`Invalid semver string: ${value}`)
}

const _parseFloat = (value: string) => {
  const p = parseFloat(value)
  if (isNaN(p)) {
    throw new commander.InvalidArgumentError(`"${value}" is not a number.`)
  }
  return p
}

const defaultRecordings = {
  flex: 'recordings/flex/zero.dat',
  senso: 'recordings/senso/zero.dat',
}

const description = `
Simulates a Senso or a Senso Flex device for testing purposes.

By default, the program simulates a Senso. The mock Senso will appear as a network
device, so the driver has to be running at the same time. The Senso can simulate
receiving firmware update commands, booting to DFU mode and receiving firmware. 

When the --flex option is used, the program will simulate a Senso Flex. When using
this mode, the application mocks the driver at the /flex endpoint, so ensure the
actual driver is not running to avoid conflicts.

Sensor data is simulated by playing back recordings made with the Driver's recording
tool. Use the recording file argument to choose the recording being replayed.
`.trim()

const program = new commander.Command()
program
  .name('mock-senso')
  .description(description)
  .usage('[options] [recording file]')
  .option('[SENSO-OPTIONS]', '(Only in Senso mode)')
  .option('--dfu', 'Boot in DFU mode', false)
  .option(
    '--fw <semver>',
    'Specify the firmware version',
    parseVersion,
    '3.3.3' as any,
  )
  .option('\n[MODE]', '')
  .option(
    '--flex',
    'Mock a Senso Flex. Note: Ensure the actual driver is not running concurrently when using this option.',
    false,
  )
  .option('\n[PLAYBACK-OPTIONS]', '(Both in Senso and Flex mode)')
  .option('--no-loop', 'Turn off looping of the recording')
  .option(
    '--speed <number>',
    'Speed of replaying the recording.',
    _parseFloat,
    1.0,
  )
  .option('\n[OTHER]', '')
  .argument(
    '[recording file]',
    `Path to the recording file to be replayed. Defaults to "${defaultRecordings.senso}" for Senso mode and
"${defaultRecordings.flex}" for Senso Flex mode if not specified.`,
  )
  .addHelpText(
    'after',
    `
Examples:
  mock-senso --no-loop --speed=0.5                    Simulate a Senso, replaying data at half speed, without looping.
  mock-senso --dfu                                    Simulate a Senso, starting in DFU mode.
  mock-senso --flex recording.dat                     Simulate a Senso Flex using a specific recording.
`,
  )
  .action((recordingFile, options) => {
    const recording =
      recordingFile ||
      (() => {
        const r = options.flex
          ? defaultRecordings.flex
          : defaultRecordings.senso
        console.log('Using default recording:', r)
        return path.resolve(__dirname, r)
      })()

    const replayOpts: Replay.Opts = {
      loop: options.loop,
      speed: options.speed,
      recording: recording,
    }

    if (options.flex) {
      return Flex.mock(replayOpts)
    }
    const sensoOpts: Senso.Opts = {
      startInDfu: options.dfu,
      firmwareVersion: options.fw,
    }
    Senso.mock(replayOpts, sensoOpts)
  })
  .parse()
