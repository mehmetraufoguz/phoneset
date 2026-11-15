import { Button, FieldGroup, Picker, Row, Spacer, Switch, Text } from "@expo/ui"
import type { ReactElement } from "react"
import type { VideoStatus } from "../core/session"
import { Session } from "../core/session"
import { modeName, TIERS, tiersOffered } from "../core/videoModes"
import { act, type useColors } from "./hooks"

type Colors = ReturnType<typeof useColors>

/** The line under the switch: why it can't be used, how it is going, or what is being sent. */
function detailOf(v: VideoStatus): { text: string; problem: boolean } {
  if (v.error) return { text: v.error, problem: true }
  if (v.unavailable) return { text: v.unavailable, problem: false }
  if (v.busy) return { text: "Starting…", problem: false }
  const r = v.report
  if (!v.on || r === null) return { text: "Off", problem: false }
  const asked = `${TIERS.find((t) => t.height === v.quality)?.name ?? `${v.quality}p`}${v.rate}`
  const used = modeName(r.mode)
  const viewers =
    r.viewers === 0
      ? "not open on the PC yet"
      : r.viewers === 1
        ? "in use on the PC"
        : `${r.viewers} viewers`
  const sent = `${used}  ·  ${((r.kBps * 8) / 1000).toFixed(1)} Mbit/s  ·  ${viewers}`
  return {
    text: r.adjusted ? `${asked} isn't available here, so ${sent}` : sent,
    problem: false
  }
}

/**
 * Everything about the camera in one place: on or off, which camera, how good a picture and how smooth, and what
 * is happening. The sizes and rates offered are what the phone's camera and the PC allow (known once the camera
 * has been on); the codec is chosen by the app and never shown. A plain function, not a component: FieldGroup
 * only lays out Sections that are its direct children.
 */
export function cameraSection(v: VideoStatus, colors: Colors): ReactElement {
  const detail = detailOf(v)
  const offered = v.report === null ? null : tiersOffered(v.report.offered)
  // Until the camera has been on nothing is known about the PC, so every planned size is listed.
  const sizes = (offered ?? TIERS.map((t) => ({ height: t.height, name: t.name, maxFps: 60 }))).map(
    (t) => ({ height: t.height, name: t.name })
  )
  // What was picked stays in the list, so the picker never shows a value that is not in it.
  if (!sizes.some((t) => t.height === v.quality)) {
    const tier = TIERS.find((t) => t.height === v.quality)
    sizes.push({ height: v.quality, name: tier?.name ?? `${v.quality}p` })
    sizes.sort((a, b) => a.height - b.height)
  }
  const sixty = offered === null || offered.some((t) => t.height === v.quality && t.maxFps >= 60)
  const rates = sixty || v.rate === 60 ? [30, 60] : [30]

  return (
    <FieldGroup.Section title="Camera">
      <Switch
        label="Use as a webcam"
        value={v.on || v.busy}
        disabled={v.busy || v.unavailable !== null}
        onValueChange={(on) => act(Session.use((s) => s.setVideo(on)))}
      />
      <Text textStyle={{ fontSize: 13, color: detail.problem ? colors.danger : colors.secondary }}>
        {detail.text}
      </Text>
      {v.unavailable === null ? (
        <>
          <Row alignment="center" spacing={8}>
            <Button
              variant={v.camera === "back" ? undefined : "outlined"}
              label="Back"
              onPress={() => act(Session.use((s) => s.setCamera("back")))}
            />
            <Button
              variant={v.camera === "front" ? undefined : "outlined"}
              label="Front"
              onPress={() => act(Session.use((s) => s.setCamera("front")))}
            />
          </Row>
          <Row alignment="center" spacing={8}>
            <Text>Quality</Text>
            <Spacer flexible />
            <Picker
              selectedValue={v.quality}
              enabled={!v.busy}
              onValueChange={(height) => act(Session.use((s) => s.setQuality(Number(height))))}
            >
              {sizes.map((t) => (
                <Picker.Item key={t.height} label={t.name} value={t.height} />
              ))}
            </Picker>
          </Row>
          <Row alignment="center" spacing={8}>
            <Text>Smoothness</Text>
            <Spacer flexible />
            <Picker
              selectedValue={v.rate}
              enabled={!v.busy}
              onValueChange={(fps) => act(Session.use((s) => s.setRate(Number(fps))))}
            >
              {rates.map((fps) => (
                <Picker.Item key={fps} label={`${fps} frames a second`} value={fps} />
              ))}
            </Picker>
          </Row>
        </>
      ) : null}
      <FieldGroup.SectionFooter>
        <Text textStyle={{ fontSize: 13, color: colors.secondary }}>
          The picture follows how you hold the phone: across for a landscape picture, upright for a
          portrait one. A bigger picture needs a faster Wi-Fi and warms the phone up. Keep this
          screen open while the camera is on. To see it on your PC, run `phoneset video` there.
        </Text>
      </FieldGroup.SectionFooter>
    </FieldGroup.Section>
  )
}
