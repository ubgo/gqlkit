// Scalar codec for the schema's `DateTime` scalar, wired in by
// cmd/generate/config.jsonc ("decode" / "encode" / "codecImport").
//
// The generated SDK calls decodeDateTime on every DateTime value in a
// response and encodeDateTime on every DateTime value it sends, so app code
// only ever sees luxon DateTime objects. null never reaches either function:
// the runtime passes null and undefined through untouched.
//
// Guide: docs/typescript/types-scalars-enums.md

import { DateTime } from "luxon";

/** Zone used to read and write timestamps; mockapi sends ISO-8601 in UTC. */
const WIRE_ZONE = "utc";

/**
 * Wire → app. Throws on anything that is not a valid ISO-8601 string; the
 * runtime wraps the throw in a ScalarCodecError naming the field path.
 */
export function decodeDateTime(raw: string): DateTime {
  if (typeof raw !== "string") throw new TypeError(`DateTime: expected an ISO-8601 string, got ${typeof raw}`);
  const parsed = DateTime.fromISO(raw, { zone: WIRE_ZONE });
  if (!parsed.isValid) throw new RangeError(`DateTime: not an ISO-8601 date-time: ${raw}`);
  return parsed;
}

/** App → wire. Sends ISO-8601 in UTC, the format mockapi stores. */
export function encodeDateTime(value: DateTime): string {
  const iso = value.setZone(WIRE_ZONE).toISO();
  if (iso === null) throw new RangeError(`DateTime: cannot encode an invalid DateTime (${value.invalidReason})`);
  return iso;
}
