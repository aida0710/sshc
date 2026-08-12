import type { HostDetail } from "../api/config";

function sameKeyword(left: string, right: string): boolean {
  return left.toLocaleLowerCase() === right.toLocaleLowerCase();
}

export function directIdentityFields(detail: HostDetail): HostDetail["form"]["fields"] {
  return detail.form.fields.filter((field) => sameKeyword(field.keyword, "IdentityFile"));
}

export function isConcreteIdentityValue(value: string): boolean {
  const trimmed = value.trim();
  return trimmed !== "" && trimmed.toLocaleLowerCase() !== "none";
}

export function hasDirectIdentityFile(detail: HostDetail): boolean {
  return directIdentityFields(detail).some((field) => field.values.some(isConcreteIdentityValue));
}
