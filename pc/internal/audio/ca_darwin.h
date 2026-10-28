// CoreAudio layer for the macOS backend. Streams run on CoreAudio's real-time
// threads and talk to Go only through a lock-free ring buffer of interleaved
// float32, so no Go code ever runs on an audio thread.

#ifndef PHONESET_CA_DARWIN_H
#define PHONESET_CA_DARWIN_H

#include <stddef.h>
#include <stdint.h>

#define CA_NAME_MAX 256

typedef struct {
	uint32_t id; // AudioObjectID
	char uid[CA_NAME_MAX];
	char name[CA_NAME_MAX];
	int in_channels;
	int out_channels;
	int is_default_in;
	int is_default_out;
} ca_device;

// Fills out with up to max devices and returns how many, or -1 on failure.
int ca_list_devices(ca_device *out, int max);

// 1 when the system-audio tap exists (macOS 14.2 or later).
int ca_tap_supported(void);

typedef struct ca_stream ca_stream;

// Plays interleaved float32 to a device's output. Feed it with ca_write.
ca_stream *ca_open_output(uint32_t device, char *err, int errlen);
// Captures a device's input. Drain it with ca_read.
ca_stream *ca_open_input(uint32_t device, char *err, int errlen);
// Captures everything the Mac plays, except this process, without a driver.
ca_stream *ca_open_system_tap(char *err, int errlen);

int ca_start(ca_stream *s, char *err, int errlen);
// Stops and releases everything, including a tap and its private aggregate device.
void ca_close(ca_stream *s);

int ca_rate(const ca_stream *s);
int ca_channels(const ca_stream *s);

// Non-blocking; counts are in frames.
size_t ca_read(ca_stream *s, float *dst, size_t frames);
size_t ca_queued(const ca_stream *s);
size_t ca_write(ca_stream *s, const float *src, size_t frames);

// Non-zero once the device went away or stopped delivering.
int ca_failed(const ca_stream *s);

#endif
