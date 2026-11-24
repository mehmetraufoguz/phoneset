import { Effect, Layer, Queue, Stream } from "effect"
import { requireOptionalNativeModule } from "expo"
import { AudioContext, AudioManager, AudioRecorder } from "react-native-audio-api"
import { AudioIO, type AudioOptions, type Player } from "../core/audio"
import { FRAME_SAMPLES, SAMPLE_RATE } from "../core/constants"
import { AudioError } from "../core/errors"
import { fadeIn, JitterTarget } from "../core/jitter"
import { Chunker, deinterleaveStereo, floatToS16, Resampler } from "../core/pcm"

const audioError = (message: string) => (cause: unknown) =>
  new AudioError({ message: cause instanceof Error ? `${message} ${cause.message}` : message })

// ------------------------------------------------------------------- session

let voiceProcessing = false
let sessionUsers = 0

// Android only: keeps Wi-Fi out of power saving while sound is streaming. With the screen off the radio otherwise hands
// frames over in groups about every 100 ms. Not present on iOS, nor in a build made before it was added.
const wifi = requireOptionalNativeModule<{ setWifiLock?(on: boolean): boolean }>("PhonesetCamera")
const holdWifi = (on: boolean) => {
  try {
    wifi?.setWifiLock?.(on)
  } catch {
    // Not worth failing the stream for.
  }
}

const applySessionOptions = Effect.try({
  try: () => {
    AudioManager.setAudioSessionOptions({
      iosCategory: "playAndRecord",
      // voiceChat turns on the system's echo cancellation and noise suppression.
      iosMode: voiceProcessing ? "voiceChat" : "default",
      iosOptions: voiceProcessing
        ? ["defaultToSpeaker", "allowBluetoothHFP", "allowBluetoothA2DP"]
        : ["defaultToSpeaker", "allowBluetoothA2DP"]
    })
  },
  catch: audioError("Couldn't set up the audio session.")
})

/**
 * Remembers the mode. While a stream is running the session is left alone: iOS rebuilds the audio
 * engine when its mode changes, and doing that under a live engine froze the app. The next stream
 * to open (see useSession) applies it.
 */
const configure = (options: AudioOptions) => {
  voiceProcessing = options.voiceProcessing
  return sessionUsers > 0 ? Effect.void : applySessionOptions
}

/** The mic and the player share one session; it stays active while either is open. */
const useSession = Effect.acquireRelease(
  Effect.gen(function* () {
    if (sessionUsers++ === 0) {
      holdWifi(true)
      yield* applySessionOptions
      yield* Effect.tryPromise({
        try: () => AudioManager.setAudioSessionActivity(true),
        catch: audioError("Couldn't start the audio session.")
      })
    }
  }),
  () =>
    Effect.promise(async () => {
      if (--sessionUsers === 0) {
        holdWifi(false)
        await AudioManager.setAudioSessionActivity(false).catch(() => {})
      }
    })
)

const ensureMicPermission = Effect.tryPromise({
  try: async () => {
    const status = await AudioManager.requestRecordingPermissions()
    if (status !== "Granted") throw new Error("denied")
  },
  catch: () =>
    new AudioError({ message: "Microphone access is off. Turn it on for phoneset in Settings." })
})

// ---------------------------------------------------------------- microphone

const mic = Stream.callback<Uint8Array, AudioError>(
  (queue) =>
    Effect.gen(function* () {
      yield* useSession

      const recorder = new AudioRecorder()
      const chunker = new Chunker(FRAME_SAMPLES * 2)
      let resampler: Resampler | undefined

      const registered = recorder.onAudioReady(
        { sampleRate: SAMPLE_RATE, bufferLength: FRAME_SAMPLES, channelCount: 1 },
        ({ buffer }) => {
          // The device may not honour 48 kHz; convert so the PC always gets what it expects.
          let samples: Float32Array = buffer.getChannelData(0)
          if (buffer.sampleRate !== SAMPLE_RATE) {
            resampler ??= new Resampler(buffer.sampleRate, SAMPLE_RATE)
            samples = resampler.process(samples)
          }
          for (const frame of chunker.push(floatToS16(samples))) Queue.offerUnsafe(queue, frame)
        }
      )
      if (registered.status === "error")
        return yield* new AudioError({ message: registered.message })

      yield* Effect.addFinalizer(() =>
        Effect.promise(async () => {
          recorder.clearOnAudioReady()
          await recorder.stop().catch(() => {})
        })
      )
      const started = yield* Effect.tryPromise({
        try: () => recorder.start(),
        catch: audioError("Couldn't start the microphone.")
      })
      if (started.status === "error") return yield* new AudioError({ message: started.message })
    }),
  // If the network can't keep up, old mic frames are worth less than new ones.
  { strategy: "sliding", bufferSize: 12 }
)

// ------------------------------------------------------------------- speaker

/** How far above the cushion the queue may grow (a burst after a stall) before the oldest audio is dropped. */
const HEADROOM_MS = 160
/** A wait this long between frames is a pause in the sound, not a late frame. */
const PAUSE_MS = 400
/** The PC sends a rush of frames when the stream opens; cutting them is not counted. */
const START_RUSH_MS = 2000
/** One frame, in milliseconds. */
const FRAME_MS = 20
/** The first 4 ms of sound after a gap rise from silence. */
const FADE_IN_SAMPLES = (SAMPLE_RATE * 4) / 1000
/** Playback speed follows the queue length by up to this much, absorbing clock drift. */
const MAX_RATE_ADJUST = 0.02

interface Queued {
  readonly id: string
  readonly samples: number
}

const openPlayer = Effect.gen(function* () {
  yield* useSession

  const context = yield* Effect.acquireRelease(
    Effect.try({
      try: () => new AudioContext({ sampleRate: SAMPLE_RATE }),
      catch: audioError("Couldn't open the speaker.")
    }),
    (c) => Effect.promise(() => c.close().catch(() => {}))
  )
  const node = yield* Effect.acquireRelease(
    Effect.try({
      try: () => {
        const n = context.createBufferQueueSource({ pitchCorrection: false })
        n.connect(context.destination)
        return n
      },
      catch: audioError("Couldn't open the speaker.")
    }),
    (n) =>
      Effect.sync(() => {
        try {
          n.stop()
        } catch {
          // already stopped
        }
      })
  )

  const jitter = new JitterTarget()
  const queue: Queued[] = []
  let queued = 0
  let started = false
  let underruns = 0
  let drops = 0
  let writes = 0
  let lastWrite = 0
  const openedAt = Date.now()
  // The fewest samples queued just before a frame came in, since the playback speed was last set.
  let lowest = Number.POSITIVE_INFINITY

  const enqueue = (l: Float32Array, r: Float32Array | null) => {
    const buffer = context.createBuffer(2, l.length, SAMPLE_RATE)
    if (r !== null) {
      buffer.copyToChannel(l as Float32Array<ArrayBuffer>, 0)
      buffer.copyToChannel(r as Float32Array<ArrayBuffer>, 1)
    }
    const id = node.enqueueBuffer(buffer)
    queue.push({ id, samples: l.length })
    queued += l.length
  }

  node.onBufferEnded = ({ bufferId }) => {
    const i = queue.findIndex((q) => q.id === bufferId)
    if (i >= 0) {
      queued -= queue[i]?.samples ?? 0
      queue.splice(i, 1)
    }
    // An empty queue is counted when the next frame arrives, in `write`: only then is it known whether the sound was
    // paused (the PC sends nothing during silence) or frames came late.
  }

  const write = (pcm: Uint8Array) =>
    Effect.try({
      try: () => {
        const now = Date.now()
        // The queue ran dry and the next frame is not a new start after a pause: a real gap in the sound.
        if (queue.length === 0 && lastWrite !== 0 && now - lastWrite <= PAUSE_MS) {
          underruns++
          jitter.underrun(now)
        }
        lastWrite = now
        jitter.arrive(now)
        const target = jitter.target(now)
        lowest = Math.min(lowest, queued)
        const [left, right] = deinterleaveStereo(pcm)
        // After silence or an underrun the queue is empty: rebuild the cushion first, and start the sound from silence.
        if (queue.length === 0) {
          enqueue(new Float32Array((SAMPLE_RATE * target) / 1000), null)
          fadeIn(left, FADE_IN_SAMPLES)
          fadeIn(right, FADE_IN_SAMPLES)
        }
        enqueue(left, right)

        // Clock drift or a burst after a stall: drop the oldest queued audio, not the playing buffer.
        while ((queued * 1000) / SAMPLE_RATE > target + HEADROOM_MS && queue.length > 2) {
          const victim = queue[1]
          if (victim === undefined) break
          node.dequeueBuffer(victim.id)
          queued -= victim.samples
          queue.splice(1, 1)
          // Frames sent in a rush when the stream opens are silence: cut, but not counted as a fault.
          if (now - openedAt > START_RUSH_MS) drops++
        }
        if (!started) {
          started = true
          // Pass the offset explicitly: the library's default (-1) trips its own range check,
          // so a bare start() always throws a RangeError.
          node.start(0, 0)
        }

        // A fuller queue plays slightly faster, an emptier one slightly slower. The level looked at is the lowest the
        // queue reached over the last 5 frames, just before a group of frames came in: measured right after a group the
        // queue is at its fullest and the cushion would settle far below what is asked for.
        if (++writes % 5 === 0) {
          const low = (lowest * 1000) / SAMPLE_RATE + FRAME_MS
          const error = (low - target) / target
          node.playbackRate.value = 1 + Math.max(-1, Math.min(1, error)) * MAX_RATE_ADJUST
          lowest = Number.POSITIVE_INFINITY
        }
      },
      catch: audioError("Couldn't play the PC's audio.")
    })

  const player: Player = {
    write,
    stats: Effect.sync(() => ({
      queuedMs: Math.round((queued * 1000) / SAMPLE_RATE),
      targetMs: jitter.target(Date.now()),
      underruns,
      drops
    }))
  }
  return player
})

export const AudioLive = Layer.succeed(AudioIO, {
  ensureMicPermission,
  configure,
  mic,
  openPlayer
})
