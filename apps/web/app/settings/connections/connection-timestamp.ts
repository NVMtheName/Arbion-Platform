const months = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
] as const;

function twoDigits(value: number) {
  return value.toString().padStart(2, "0");
}

// This intentionally avoids environment-dependent locale and timezone defaults.
// Connections are server-rendered and then hydrated in the owner's browser, so
// both environments must produce byte-for-byte identical timestamp text.
export function formatConnectionTimestamp(value: string) {
  const instant = new Date(value);
  if (Number.isNaN(instant.valueOf())) return "UNAVAILABLE";
  const hour = instant.getUTCHours();
  const displayHour = hour % 12 || 12;
  const meridiem = hour < 12 ? "AM" : "PM";
  return `${months[instant.getUTCMonth()]} ${instant.getUTCDate()}, ${instant.getUTCFullYear()}, ${displayHour}:${twoDigits(instant.getUTCMinutes())}:${twoDigits(instant.getUTCSeconds())} ${meridiem} UTC`;
}
