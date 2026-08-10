import type { HostDetail } from "../api/config";
import { formatValues } from "./values";

export type BasicKeyword = "HostName" | "User" | "Port";

export type BasicFieldState = {
  keyword: BasicKeyword;
  value: string;
  origin: "direct" | "inherited" | "default" | "complex";
  source?: { path?: string; absolute?: string; line?: number };
  editable: boolean;
};

function sameKeyword(left: string, right: string): boolean {
  return left.toLocaleLowerCase() === right.toLocaleLowerCase();
}

function defaultValue(detail: HostDetail, keyword: BasicKeyword): string {
  switch (keyword) {
    case "HostName": return detail.form.entry.identity.alias;
    case "User": return "";
    case "Port": return "22";
  }
}

export function deriveBasicField(detail: HostDetail, keyword: BasicKeyword): BasicFieldState {
  const direct = detail.form.fields.filter((field) => sameKeyword(field.keyword, keyword));
  if (direct.length > 1) {
    return {
      keyword,
      value: direct.map((field) => formatValues(field.values)).join("  /  "),
      origin: "complex",
      editable: false,
    };
  }
  if (direct.length === 1) {
    const field = direct[0]!;
    return {
      keyword,
      value: formatValues(field.values),
      origin: "direct",
      source: detail.form.entry.file.path === undefined
        ? { absolute: detail.form.entry.file.absolute, line: field.line }
        : { path: detail.form.entry.file.path, line: field.line },
      editable: field.editable,
    };
  }
  const inherited = detail.effective.entries.find((entry) => sameKeyword(entry.keyword, keyword));
  if (inherited !== undefined) {
    return {
      keyword,
      value: formatValues(inherited.values),
      origin: "inherited",
      source: inherited.source,
      editable: detail.form.entry.editable,
    };
  }
  return {
    keyword,
    value: defaultValue(detail, keyword),
    origin: "default",
    editable: detail.form.entry.editable,
  };
}
