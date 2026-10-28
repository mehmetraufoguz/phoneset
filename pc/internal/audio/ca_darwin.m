//go:build darwin && cgo

#import <AudioToolbox/AudioToolbox.h>
#import <CoreAudio/AudioHardwareTapping.h>
#import <CoreAudio/CATapDescription.h>
#import <CoreAudio/CoreAudio.h>
#import <Foundation/Foundation.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "ca_darwin.h"

// ------------------------------------------------------------------- ring

// Single producer, single consumer. Indices only ever grow; the difference is the fill.
typedef struct {
	float *buf;
	size_t cap; // floats
	_Atomic size_t head;
	_Atomic size_t tail;
} ring_t;

static int ring_init(ring_t *r, size_t cap) {
	r->buf = calloc(cap, sizeof(float));
	r->cap = cap;
	atomic_init(&r->head, 0);
	atomic_init(&r->tail, 0);
	return r->buf != NULL;
}

static size_t ring_used(const ring_t *r) {
	size_t head = atomic_load_explicit(&r->head, memory_order_acquire);
	size_t tail = atomic_load_explicit(&r->tail, memory_order_acquire);
	return head - tail;
}

static size_t ring_write(ring_t *r, const float *src, size_t n) {
	size_t head = atomic_load_explicit(&r->head, memory_order_relaxed);
	size_t tail = atomic_load_explicit(&r->tail, memory_order_acquire);
	size_t space = r->cap - (head - tail);
	if (n > space) {
		n = space;
	}
	for (size_t i = 0; i < n; i++) {
		r->buf[(head + i) % r->cap] = src[i];
	}
	atomic_store_explicit(&r->head, head + n, memory_order_release);
	return n;
}

static size_t ring_read(ring_t *r, float *dst, size_t n) {
	size_t tail = atomic_load_explicit(&r->tail, memory_order_relaxed);
	size_t head = atomic_load_explicit(&r->head, memory_order_acquire);
	size_t used = head - tail;
	if (n > used) {
		n = used;
	}
	for (size_t i = 0; i < n; i++) {
		dst[i] = r->buf[(tail + i) % r->cap];
	}
	atomic_store_explicit(&r->tail, tail + n, memory_order_release);
	return n;
}

// ------------------------------------------------------------------ stream

struct ca_stream {
	AudioUnit unit;
	int input;
	AudioObjectID device;
	int rate;
	int channels;
	ring_t ring;
	float *scratch;
	UInt32 scratch_frames;
	_Atomic int failed;
	AudioObjectID tap;
	AudioObjectID aggregate;
	int listener;
};

static void fail(char *err, int errlen, const char *what, OSStatus st) {
	if (err && errlen > 0) {
		snprintf(err, (size_t)errlen, "%s (CoreAudio error %d)", what, (int)st);
	}
}

static void cfstring(CFStringRef s, char *dst, int n) {
	dst[0] = 0;
	if (s == NULL) {
		return;
	}
	if (!CFStringGetCString(s, dst, n, kCFStringEncodingUTF8)) {
		dst[0] = 0;
	}
	CFRelease(s);
}

static AudioObjectPropertyAddress addr(AudioObjectPropertySelector sel, AudioObjectPropertyScope scope) {
	AudioObjectPropertyAddress a = {sel, scope, kAudioObjectPropertyElementMain};
	return a;
}

static int device_channels(AudioObjectID dev, AudioObjectPropertyScope scope) {
	AudioObjectPropertyAddress a = addr(kAudioDevicePropertyStreamConfiguration, scope);
	UInt32 size = 0;
	if (AudioObjectGetPropertyDataSize(dev, &a, 0, NULL, &size) != noErr || size == 0) {
		return 0;
	}
	AudioBufferList *list = malloc(size);
	int total = 0;
	if (AudioObjectGetPropertyData(dev, &a, 0, NULL, &size, list) == noErr) {
		for (UInt32 i = 0; i < list->mNumberBuffers; i++) {
			total += (int)list->mBuffers[i].mNumberChannels;
		}
	}
	free(list);
	return total;
}

static AudioObjectID default_device(AudioObjectPropertySelector sel) {
	AudioObjectID id = kAudioObjectUnknown;
	UInt32 size = sizeof(id);
	AudioObjectPropertyAddress a = addr(sel, kAudioObjectPropertyScopeGlobal);
	if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, &id) != noErr) {
		return kAudioObjectUnknown;
	}
	return id;
}

int ca_list_devices(ca_device *out, int max) {
	AudioObjectPropertyAddress a = addr(kAudioHardwarePropertyDevices, kAudioObjectPropertyScopeGlobal);
	UInt32 size = 0;
	if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &a, 0, NULL, &size) != noErr) {
		return -1;
	}
	int count = (int)(size / sizeof(AudioObjectID));
	AudioObjectID *ids = malloc(size);
	if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, ids) != noErr) {
		free(ids);
		return -1;
	}
	AudioObjectID def_in = default_device(kAudioHardwarePropertyDefaultInputDevice);
	AudioObjectID def_out = default_device(kAudioHardwarePropertyDefaultOutputDevice);

	int n = 0;
	for (int i = 0; i < count && n < max; i++) {
		ca_device *d = &out[n];
		memset(d, 0, sizeof(*d));
		d->id = ids[i];
		d->in_channels = device_channels(ids[i], kAudioObjectPropertyScopeInput);
		d->out_channels = device_channels(ids[i], kAudioObjectPropertyScopeOutput);
		if (d->in_channels == 0 && d->out_channels == 0) {
			continue;
		}
		CFStringRef name = NULL, uid = NULL;
		UInt32 sz = sizeof(name);
		AudioObjectPropertyAddress na = addr(kAudioObjectPropertyName, kAudioObjectPropertyScopeGlobal);
		if (AudioObjectGetPropertyData(ids[i], &na, 0, NULL, &sz, &name) == noErr) {
			cfstring(name, d->name, CA_NAME_MAX);
		}
		sz = sizeof(uid);
		AudioObjectPropertyAddress ua = addr(kAudioDevicePropertyDeviceUID, kAudioObjectPropertyScopeGlobal);
		if (AudioObjectGetPropertyData(ids[i], &ua, 0, NULL, &sz, &uid) == noErr) {
			cfstring(uid, d->uid, CA_NAME_MAX);
		}
		d->is_default_in = ids[i] == def_in;
		d->is_default_out = ids[i] == def_out;
		n++;
	}
	free(ids);
	return n;
}

int ca_tap_supported(void) {
	if (@available(macOS 14.2, *)) {
		return 1;
	}
	return 0;
}

// ---------------------------------------------------------------- callbacks

static OSStatus render_cb(void *ref, AudioUnitRenderActionFlags *flags, const AudioTimeStamp *ts,
						  UInt32 bus, UInt32 frames, AudioBufferList *io) {
	ca_stream *s = ref;
	float *dst = io->mBuffers[0].mData;
	size_t want = (size_t)frames * (size_t)s->channels;
	size_t got = ring_read(&s->ring, dst, want);
	if (got < want) {
		memset(dst + got, 0, (want - got) * sizeof(float));
		if (got == 0) {
			*flags |= kAudioUnitRenderAction_OutputIsSilence;
		}
	}
	return noErr;
}

static OSStatus input_cb(void *ref, AudioUnitRenderActionFlags *flags, const AudioTimeStamp *ts,
						 UInt32 bus, UInt32 frames, AudioBufferList *unused) {
	ca_stream *s = ref;
	if (frames > s->scratch_frames) {
		return noErr; // larger than anything we sized for; drop rather than overrun
	}
	AudioBufferList list;
	list.mNumberBuffers = 1;
	list.mBuffers[0].mNumberChannels = (UInt32)s->channels;
	list.mBuffers[0].mDataByteSize = frames * (UInt32)s->channels * sizeof(float);
	list.mBuffers[0].mData = s->scratch;
	OSStatus st = AudioUnitRender(s->unit, flags, ts, bus, frames, &list);
	if (st != noErr) {
		return st;
	}
	ring_write(&s->ring, s->scratch, (size_t)frames * (size_t)s->channels);
	return noErr;
}

static OSStatus alive_cb(AudioObjectID obj, UInt32 n, const AudioObjectPropertyAddress *addrs, void *ref) {
	ca_stream *s = ref;
	UInt32 alive = 1;
	UInt32 size = sizeof(alive);
	AudioObjectPropertyAddress a = addr(kAudioDevicePropertyDeviceIsAlive, kAudioObjectPropertyScopeGlobal);
	if (AudioObjectGetPropertyData(obj, &a, 0, NULL, &size, &alive) != noErr || !alive) {
		atomic_store(&s->failed, 1);
	}
	return noErr;
}

// -------------------------------------------------------------------- open

static void release(ca_stream *s) {
	if (s == NULL) {
		return;
	}
	if (s->listener) {
		AudioObjectPropertyAddress a = addr(kAudioDevicePropertyDeviceIsAlive, kAudioObjectPropertyScopeGlobal);
		AudioObjectRemovePropertyListener(s->device, &a, alive_cb, s);
	}
	if (s->unit) {
		AudioOutputUnitStop(s->unit);
		AudioUnitUninitialize(s->unit);
		AudioComponentInstanceDispose(s->unit);
	}
	if (@available(macOS 14.2, *)) {
		if (s->aggregate) {
			AudioHardwareDestroyAggregateDevice(s->aggregate);
		}
		if (s->tap) {
			AudioHardwareDestroyProcessTap(s->tap);
		}
	}
	free(s->ring.buf);
	free(s->scratch);
	free(s);
}

static ca_stream *open_unit(AudioObjectID dev, int input, char *err, int errlen) {
	ca_stream *s = calloc(1, sizeof(*s));
	s->input = input;
	s->device = dev;
	atomic_init(&s->failed, 0);

	AudioComponentDescription desc = {
		kAudioUnitType_Output, kAudioUnitSubType_HALOutput, kAudioUnitManufacturer_Apple, 0, 0};
	AudioComponent comp = AudioComponentFindNext(NULL, &desc);
	OSStatus st = comp ? AudioComponentInstanceNew(comp, &s->unit) : -1;
	if (st != noErr) {
		fail(err, errlen, "Couldn't create the audio unit", st);
		s->unit = NULL;
		release(s);
		return NULL;
	}

	UInt32 on = 1, off = 0;
	st = AudioUnitSetProperty(s->unit, kAudioOutputUnitProperty_EnableIO, kAudioUnitScope_Input, 1,
							  input ? &on : &off, sizeof(on));
	if (st == noErr) {
		st = AudioUnitSetProperty(s->unit, kAudioOutputUnitProperty_EnableIO, kAudioUnitScope_Output, 0,
								  input ? &off : &on, sizeof(on));
	}
	if (st == noErr) {
		st = AudioUnitSetProperty(s->unit, kAudioOutputUnitProperty_CurrentDevice, kAudioUnitScope_Global, 0,
								  &dev, sizeof(dev));
	}
	if (st != noErr) {
		fail(err, errlen, "Couldn't select the audio device", st);
		release(s);
		return NULL;
	}

	// Run at the device's own rate and channel count; Go converts.
	AudioStreamBasicDescription fmt;
	UInt32 size = sizeof(fmt);
	if (input) {
		st = AudioUnitGetProperty(s->unit, kAudioUnitProperty_StreamFormat, kAudioUnitScope_Input, 1, &fmt, &size);
		if (st != noErr) {
			fail(err, errlen, "Couldn't read the input format", st);
			release(s);
			return NULL;
		}
		s->rate = (int)fmt.mSampleRate;
		s->channels = (int)fmt.mChannelsPerFrame;
	} else {
		Float64 rate = 0;
		UInt32 rsz = sizeof(rate);
		AudioObjectPropertyAddress ra = addr(kAudioDevicePropertyNominalSampleRate, kAudioObjectPropertyScopeGlobal);
		st = AudioObjectGetPropertyData(dev, &ra, 0, NULL, &rsz, &rate);
		if (st != noErr || rate <= 0) {
			fail(err, errlen, "Couldn't read the output sample rate", st);
			release(s);
			return NULL;
		}
		s->rate = (int)rate;
		s->channels = device_channels(dev, kAudioObjectPropertyScopeOutput);
	}
	if (s->rate <= 0 || s->channels <= 0) {
		fail(err, errlen, "The device has no usable audio format", -1);
		release(s);
		return NULL;
	}

	AudioStreamBasicDescription client = {0};
	client.mSampleRate = s->rate;
	client.mFormatID = kAudioFormatLinearPCM;
	client.mFormatFlags = kAudioFormatFlagIsFloat | kAudioFormatFlagIsPacked;
	client.mChannelsPerFrame = (UInt32)s->channels;
	client.mBitsPerChannel = 32;
	client.mFramesPerPacket = 1;
	client.mBytesPerFrame = client.mBitsPerChannel / 8 * client.mChannelsPerFrame;
	client.mBytesPerPacket = client.mBytesPerFrame;
	st = AudioUnitSetProperty(s->unit, kAudioUnitProperty_StreamFormat,
							  input ? kAudioUnitScope_Output : kAudioUnitScope_Input, input ? 1 : 0, &client,
							  sizeof(client));
	if (st != noErr) {
		fail(err, errlen, "Couldn't set the audio format", st);
		release(s);
		return NULL;
	}

	AURenderCallbackStruct cb = {input ? input_cb : render_cb, s};
	st = AudioUnitSetProperty(s->unit,
							  input ? kAudioOutputUnitProperty_SetInputCallback : kAudioUnitProperty_SetRenderCallback,
							  input ? kAudioUnitScope_Global : kAudioUnitScope_Input, 0, &cb, sizeof(cb));
	if (st != noErr) {
		fail(err, errlen, "Couldn't set the audio callback", st);
		release(s);
		return NULL;
	}

	if (input) {
		UInt32 max = 4096;
		UInt32 msz = sizeof(max);
		AudioUnitGetProperty(s->unit, kAudioUnitProperty_MaximumFramesPerSlice, kAudioUnitScope_Global, 0, &max, &msz);
		s->scratch_frames = max < 4096 ? 4096 : max;
		s->scratch = malloc((size_t)s->scratch_frames * (size_t)s->channels * sizeof(float));
	}
	if (!ring_init(&s->ring, (size_t)s->rate * (size_t)s->channels) || (input && !s->scratch)) {
		fail(err, errlen, "Out of memory", -1);
		release(s);
		return NULL;
	}

	st = AudioUnitInitialize(s->unit);
	if (st != noErr) {
		fail(err, errlen, "Couldn't initialise the audio unit", st);
		AudioComponentInstanceDispose(s->unit);
		s->unit = NULL;
		release(s);
		return NULL;
	}

	AudioObjectPropertyAddress la = addr(kAudioDevicePropertyDeviceIsAlive, kAudioObjectPropertyScopeGlobal);
	if (AudioObjectAddPropertyListener(dev, &la, alive_cb, s) == noErr) {
		s->listener = 1;
	}
	return s;
}

ca_stream *ca_open_output(uint32_t device, char *err, int errlen) {
	return open_unit(device, 0, err, errlen);
}

ca_stream *ca_open_input(uint32_t device, char *err, int errlen) {
	return open_unit(device, 1, err, errlen);
}

ca_stream *ca_open_system_tap(char *err, int errlen) {
	if (@available(macOS 14.2, *)) {
		// Leave this process out, or the phone's own microphone would be sent straight back.
		AudioObjectID self = kAudioObjectUnknown;
		pid_t pid = getpid();
		UInt32 sz = sizeof(self);
		AudioObjectPropertyAddress pa = addr(kAudioHardwarePropertyTranslatePIDToProcessObject, kAudioObjectPropertyScopeGlobal);
		if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &pa, sizeof(pid), &pid, &sz, &self) != noErr) {
			self = kAudioObjectUnknown;
		}
		NSArray<NSNumber *> *excluded = self != kAudioObjectUnknown ? @[ @(self) ] : @[];

		CATapDescription *desc = [[CATapDescription alloc] initStereoGlobalTapButExcludeProcesses:excluded];
		desc.name = @"phoneset system audio";
		desc.privateTap = YES;
		desc.muteBehavior = CATapUnmuted;

		// Bind the tap to the default output device's stream, as Chromium's tap capture does.
		// That also gives the aggregate device a clock that runs with the speakers.
		AudioObjectID output = default_device(kAudioHardwarePropertyDefaultOutputDevice);
		if (output != kAudioObjectUnknown) {
			CFStringRef outUID = NULL;
			UInt32 usz = sizeof(outUID);
			AudioObjectPropertyAddress ua = addr(kAudioDevicePropertyDeviceUID, kAudioObjectPropertyScopeGlobal);
			if (AudioObjectGetPropertyData(output, &ua, 0, NULL, &usz, &outUID) == noErr && outUID != NULL) {
				desc.deviceUID = (__bridge NSString *)outUID;
				desc.stream = @0;
				CFRelease(outUID);
			}
		}

		AudioObjectID tap = kAudioObjectUnknown;
		OSStatus st = AudioHardwareCreateProcessTap(desc, &tap);
		if (st != noErr) {
			fail(err, errlen, "Couldn't tap the system audio", st);
			return NULL;
		}

		NSDictionary *aggregate = @{
			@kAudioAggregateDeviceNameKey : @"phoneset tap",
			@kAudioAggregateDeviceUIDKey : [[NSUUID UUID] UUIDString],
			@kAudioAggregateDeviceIsPrivateKey : @YES,
			@kAudioAggregateDeviceTapAutoStartKey : @YES,
			@kAudioAggregateDeviceTapListKey : @[ @{
				@kAudioSubTapUIDKey : [desc.UUID UUIDString],
				@kAudioSubTapDriftCompensationKey : @YES,
			} ],
		};
		AudioObjectID agg = kAudioObjectUnknown;
		st = AudioHardwareCreateAggregateDevice((__bridge CFDictionaryRef)aggregate, &agg);
		if (st != noErr) {
			AudioHardwareDestroyProcessTap(tap);
			fail(err, errlen, "Couldn't create the capture device for the tap", st);
			return NULL;
		}

		ca_stream *s = open_unit(agg, 1, err, errlen);
		if (s == NULL) {
			AudioHardwareDestroyAggregateDevice(agg);
			AudioHardwareDestroyProcessTap(tap);
			return NULL;
		}
		s->tap = tap;
		s->aggregate = agg;
		return s;
	}
	fail(err, errlen, "System audio capture needs macOS 14.2 or later", -1);
	return NULL;
}

int ca_start(ca_stream *s, char *err, int errlen) {
	OSStatus st = AudioOutputUnitStart(s->unit);
	if (st != noErr) {
		fail(err, errlen, "Couldn't start the audio stream", st);
		return -1;
	}
	return 0;
}

void ca_close(ca_stream *s) { release(s); }

int ca_rate(const ca_stream *s) { return s->rate; }
int ca_channels(const ca_stream *s) { return s->channels; }

size_t ca_read(ca_stream *s, float *dst, size_t frames) {
	size_t ch = (size_t)s->channels;
	return ring_read(&s->ring, dst, frames * ch) / ch;
}

size_t ca_queued(const ca_stream *s) { return ring_used(&s->ring) / (size_t)s->channels; }

size_t ca_write(ca_stream *s, const float *src, size_t frames) {
	size_t ch = (size_t)s->channels;
	return ring_write(&s->ring, src, frames * ch) / ch;
}

int ca_failed(const ca_stream *s) { return atomic_load(&((ca_stream *)s)->failed); }
