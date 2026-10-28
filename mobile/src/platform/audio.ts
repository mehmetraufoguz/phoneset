import { Effect, Layer, Queue, Stream } from "effect"
import { Platform } from "react-native"
import {
  AudioContext,
  AudioManager,
  AudioRecorder,
  RecordingNotificationManager
} from "react-native-audio-api"
import { AudioIO, type AudioOptions, type Player } from "../core/audio"
import { FRAME_SAMPLES, SAMPLE_RATE } from "../core/constants"
import { AudioError } from "../core/errors"
import { Chunker, deinterleaveStereo, floatToS16, Resampler } from "../core/pcm"

const audioError = (message: string) => (cause: unknown) =>
  new AudioError({ message: cause instanceof Error ? `${message} ${cause.message}` : message })

// ------------------------------------------------------------------- session

let voiceProcessing = false
let sessionUsers = 0

const configure = (options: AudioOptions) =>
  Effect.try({
    try: () => {
      voiceProcessing = options.voiceProcessing
      AudioManager.setAudioSessionOptions({
        iosCategory: "playAndRecord",
        // voiceChat turns on the system's echo cancellation and noise suppression.
        iosMode: options.voiceProcessing ? "voiceChat" : "default",
        iosOptions: options.voiceProcessing
          ? ["defaultToSpeaker", "allowBluetoothHFP", "allowBluetoothA2DP"]
          : ["defaultToSpeaker", "allowBluetoothA2DP"]
      })
    },
    catch: audioError("Couldn't set up the audio session.")
  })

/** The mic and the player share one session; it stays active while either is open. */
const useSession = Effect.acquireRelease(
  Effect.gen(function* () {
    if (sessionUsers++ === 0) {
      yield* configure({ voiceProcessing })
      yield* Effect.tryPromise({
        try: () => AudioManager.setAudioSessionActivity(true),
        catch: audioError("Couldn't start the audio session.")
      })
    }
  }),
  () =>
    Effect.promise(async () => {
      if (--sessionUsers === 0) await AudioManager.setAudioSessionActivity(false).catch(() => {})
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

/** On Android a foreground-service notification keeps the mic alive in the background. */
const keepAliveNotification = Effect.acquireRelease(
  Platform.OS === "android"
    ? Effect.promise(async () => {
        await AudioManager.requestNotificationPermissions().catch(() => {})
        await RecordingNotificationManager.show({
          title: "phoneset",
          contentText: "Your phone is the PC's microphone"
        }).catch(() => {})
      })
    : Effect.void,
  () =>
    Platform.OS === "android"
      ? Effect.promise(() => RecordingNotificationManager.hide().catch(() => {}))
      : Effect.void
)

const mic = Stream.callback<Uint8Array, AudioError>(
  (queue) =>
    Effect.gen(function* () {
      yield* useSession
      yield* keepAliveNotification

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

const CUSHION_MS = 60
const MAX_QUEUED_MS = 220
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

  const queue: Queued[] = []
  let queued = 0
  let started = false
  let underruns = 0
  let drops = 0
  let writes = 0

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

  node.onBufferEnded = ({ bufferId, isLastBufferInQueue }) => {
    const i = queue.findIndex((q) => q.id === bufferId)
    if (i >= 0) {
      queued -= queue[i]?.samples ?? 0
      queue.splice(i, 1)
    }
    if (isLastBufferInQueue && queue.length === 0) underruns++
  }

  const write = (pcm: Uint8Array) =>
    Effect.try({
      try: () => {
        // After silence or an underrun the queue is empty: rebuild the cushion first.
        if (queue.length === 0) {
          enqueue(new Float32Array((SAMPLE_RATE * CUSHION_MS) / 1000), null)
        }
        const [left, right] = deinterleaveStereo(pcm)
        enqueue(left, right)

        // Clock drift or a burst after a stall: drop the oldest queued audio, not the playing buffer.
        while ((queued * 1000) / SAMPLE_RATE > MAX_QUEUED_MS && queue.length > 2) {
          const victim = queue[1]
          if (victim === undefined) break
          node.dequeueBuffer(victim.id)
          queued -= victim.samples
          queue.splice(1, 1)
          drops++
        }
        if (!started) {
          started = true
          // Pass the offset explicitly: the library's default (-1) trips its own range check,
          // so a bare start() always throws a RangeError.
          node.start(0, 0)
        }

        // A fuller queue plays slightly faster, an emptier one slightly slower.
        if (++writes % 5 === 0) {
          const error = ((queued * 1000) / SAMPLE_RATE - CUSHION_MS) / CUSHION_MS
          node.playbackRate.value = 1 + Math.max(-1, Math.min(1, error)) * MAX_RATE_ADJUST
        }
      },
      catch: audioError("Couldn't play the PC's audio.")
    })

  const player: Player = {
    write,
    stats: Effect.sync(() => ({
      queuedMs: Math.round((queued * 1000) / SAMPLE_RATE),
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
