// Deflate expands its input at most about 1032 times, so feeding one KiB at a
// time lets at most about one MiB of output pile up before it is counted.
const inputSliceBytes = 1024;

export type InflatedSizeMeter = {
  // push inflates more of the zlib stream and resolves false once the output
  // has grown past the limit or the stream is not valid zlib.
  push(compressed: Uint8Array<ArrayBuffer>): Promise<boolean>;
  // finish ends the stream and resolves true only when the whole stream was
  // valid and its output stayed within the limit.
  finish(): Promise<boolean>;
  // head holds the first bytes of the output, up to the requested length.
  head(): Uint8Array;
  cancel(): void;
};

// createInflatedSizeMeter counts the bytes a zlib stream (RFC 1950) inflates
// to without keeping them, so a small compressed payload can be refused before
// anything inflates all of it into memory.
export function createInflatedSizeMeter({ limit, headLength }: { limit: number; headLength: number }): InflatedSizeMeter {
  const stream = new DecompressionStream("deflate");
  const writer = stream.writable.getWriter();
  const reader = stream.readable.getReader();
  const head = new Uint8Array(headLength);
  let headFilled = 0;
  let inflated = 0;
  let failed = false;

  const fail = () => {
    failed = true;
    writer.abort().catch(() => undefined);
    reader.cancel().catch(() => undefined);
  };

  const drained = (async () => {
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) return;
        if (headFilled < headLength) {
          const copied = value.subarray(0, headLength - headFilled);
          head.set(copied, headFilled);
          headFilled += copied.byteLength;
        }
        inflated += value.byteLength;
        if (inflated > limit) {
          fail();
          return;
        }
      }
    } catch {
      fail();
    }
  })();

  return {
    async push(compressed) {
      for (let offset = 0; offset < compressed.byteLength && !failed; offset += inputSliceBytes) {
        try {
          await writer.write(compressed.subarray(offset, offset + inputSliceBytes));
        } catch {
          fail();
        }
      }
      return !failed;
    },
    async finish() {
      if (!failed) {
        try {
          await writer.close();
        } catch {
          fail();
        }
      }
      await drained;
      return !failed;
    },
    head: () => head.subarray(0, headFilled),
    cancel: fail,
  };
}
