import type { About } from "../api/about";
import { useLanguage } from "../i18n/context";
import { FactRows, type Fact } from "../ui/FactRows";
import { formatDateTime } from "../ui/format";

// Seven characters keep the revision readable while the link uses the complete SHA.
const shortRevisionLength = 7;

// BuildDetails describes the running executable. A build without version control
// information (Homebrew builds a source archive without .git) leaves out the rows
// it cannot fill instead of reading "Unknown".
export function BuildDetails({ about }: { about: About }) {
  const { t, locale } = useLanguage();
  const commitURL = /^[a-f0-9]{40,64}$/i.test(about.revision) ? `https://github.com/aida0710/sshc/commit/${about.revision}` : undefined;
  const fields: Fact[] = [{ label: t("about.version"), value: about.version }];
  if (commitURL !== undefined) {
    fields.push({ label: t("about.commit"), value: <a href={commitURL} target="_blank" rel="noopener noreferrer" className="text-accent underline">{about.revision.slice(0, shortRevisionLength)}</a> });
  }
  if (about.commitTime !== undefined) {
    fields.push({ label: t("about.commitTime"), value: formatDateTime(about.commitTime, locale) });
  }
  if (about.modified !== undefined) {
    fields.push({ label: t("about.modified"), value: t(about.modified ? "about.modifiedYes" : "about.modifiedNo") });
  }
  fields.push(
    { label: t("about.goVersion"), value: about.goVersion },
    { label: t("about.platform"), value: `${about.os} / ${about.arch}` },
    { label: t("about.manager"), value: about.manager === "unknown" ? t("about.unmanaged") : about.manager === "homebrew" ? "Homebrew" : about.manager },
  );
  if (about.executable !== "") {
    fields.push({ label: t("about.executable"), value: about.executable });
  }
  return <FactRows facts={fields} />;
}
