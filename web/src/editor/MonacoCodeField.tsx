// Load this module only through loadMonacoCodeField (loadMonacoEditor.ts): Monaco creates its Trusted Types policies
// through MonacoEnvironment while the imports below are evaluated, so the environment must be installed first.
import { useEffect, useRef, useState } from "react";
import * as monaco from "monaco-editor/editor/editor.api.js";
import "./monacoEditorFeatures";
import "monaco-editor/languages/definitions/shell/register.js";
import { useTheme } from "../theme/context";
import { editorCursorBlinking, reducedMotionQuery } from "../ui/reducedMotion";
import { useMediaQuery } from "../ui/useMediaQuery";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";
import { codeFieldFrame, type CodeMarker, type LoadedCodeFieldProps } from "./CodeField";
import { editorFontFamily } from "./editorFont";
import { noUnrequestedEditOptions, startWithTabIndenting } from "./monacoEditing";
import { registerSSHConfigLanguage, sshConfigLanguageId } from "./sshConfigLanguage";
import { useModelFollowsValue } from "./useModelFollowsValue";

// The field's text is set as the other monospace fields are (12.5px), with a line a little taller than the text.
const fontSize = 12.5;
const lineHeight = 19;
// verticalPadding is the room above the first line and below the last.
const verticalPadding = 8;

// markerOwner names sshc's markers among Monaco's.
const markerOwner = "sshc";

const markerSeverities: Record<CodeMarker["severity"], monaco.MarkerSeverity> = {
  error: monaco.MarkerSeverity.Error,
  warning: monaco.MarkerSeverity.Warning,
  info: monaco.MarkerSeverity.Info,
};

function linesHeight(lines: number): number {
  return lines * lineHeight + verticalPadding * 2;
}

// MonacoCodeField is CodeField once Monaco has loaded. An SSH config is edited as a file is: line numbers, and Tab
// indents (ssh_config is written indented). The other fields are form fields: Tab moves to the next field.
export function MonacoCodeField({
  label, value, onChange, onBlur, language = "plaintext", disabled = false, placeholder, minLines = 3, maxLines = 12,
  fill = false, markers = [], selectedLine = null, typingBeforeLoad, ...description
}: LoadedCodeFieldProps) {
  const container = useRef<HTMLDivElement>(null);
  const editor = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
  const reports = useRef<string[]>([]);
  const change = useRef(onChange);
  const leave = useRef(onBlur);
  change.current = onChange;
  leave.current = onBlur;
  const [height, setHeight] = useState(linesHeight(minLines));
  const { scheme } = useTheme();
  const reducedMotion = useMediaQuery(reducedMotionQuery);
  const sshConfig = language === "ssh-config";

  useEffect(() => {
    if (container.current === null) return;
    if (sshConfig) registerSSHConfigLanguage();
    const model = monaco.editor.createModel(value, sshConfig ? sshConfigLanguageId : language);
    const view = monaco.editor.create(container.current, {
      model,
      ...noUnrequestedEditOptions,
      ariaLabel: label,
      readOnly: disabled,
      placeholder,
      automaticLayout: true,
      // The field is often a few lines tall in a framed box; its pop-ups (a problem's hover, the keywords offered)
      // are drawn over the page so they are not cut off at the box's edge.
      fixedOverflowWidgets: true,
      minimap: { enabled: false },
      fontFamily: editorFontFamily,
      fontSize,
      lineHeight,
      lineNumbers: sshConfig ? "on" : "off",
      // A config file keeps its lines as they are, the way the line numbers count them. The other fields wrap a long
      // line (a command, a key in a VPN's file) rather than hide its end beyond the field's edge.
      wordWrap: sshConfig ? "off" : "on",
      lineNumbersMinChars: 3,
      glyphMargin: false,
      folding: false,
      renderLineHighlight: "none",
      overviewRulerLanes: 0,
      hideCursorInOverviewRuler: true,
      scrollbar: { alwaysConsumeMouseWheel: false },
      padding: { top: verticalPadding, bottom: verticalPadding },
      scrollBeyondLastLine: false,
      tabFocusMode: !sshConfig,
      cursorBlinking: editorCursorBlinking(reducedMotion),
      theme: scheme === "dark" ? "vs-dark" : "vs",
    });
    if (sshConfig) startWithTabIndenting(view);
    editor.current = view;
    if (typingBeforeLoad?.current === true) {
      view.setPosition(model.getFullModelRange().getEndPosition());
      view.focus();
    }
    reports.current = [];
    const subscriptions = [
      model.onDidChangeContent(() => {
        const next = model.getValue();
        reports.current.push(next);
        change.current(next);
      }),
      view.onDidBlurEditorText(() => leave.current?.()),
      view.onDidContentSizeChange((event) => {
        if (!fill) setHeight(Math.min(Math.max(event.contentHeight, linesHeight(minLines)), linesHeight(maxLines)));
      }),
    ];
    return () => {
      subscriptions.forEach((subscription) => subscription.dispose());
      view.dispose();
      model.dispose();
      editor.current = null;
    };
    // The editor is made once; the effects below follow the props that change.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useModelFollowsValue(editor, value, reports);

  useEffect(() => {
    editor.current?.updateOptions({
      ariaLabel: label,
      readOnly: disabled,
      placeholder,
      cursorBlinking: editorCursorBlinking(reducedMotion),
      theme: scheme === "dark" ? "vs-dark" : "vs",
    });
  }, [disabled, label, placeholder, reducedMotion, scheme]);

  // The row's hint and warning describe the element Monaco reads input from, as they would a text area: the textbox
  // of the browser's EditContext where it has one (Chromium), else Monaco's text area.
  const describedBy = description["aria-describedby"];
  const invalid = description["aria-invalid"];
  useEffect(() => {
    const input = container.current?.querySelector('[role="textbox"]') ?? container.current?.querySelector("textarea");
    if (input === null || input === undefined) return;
    if (describedBy === undefined) input.removeAttribute("aria-describedby");
    else input.setAttribute("aria-describedby", describedBy);
    if (invalid === true) input.setAttribute("aria-invalid", "true");
    else input.removeAttribute("aria-invalid");
  }, [describedBy, invalid]);

  useEffect(() => {
    const model = editor.current?.getModel();
    if (model === null || model === undefined) return;
    monaco.editor.setModelMarkers(model, markerOwner, markers.filter((marker) => marker.line <= model.getLineCount()).map((marker) => ({
      severity: markerSeverities[marker.severity],
      message: marker.message,
      startLineNumber: marker.line,
      startColumn: model.getLineFirstNonWhitespaceColumn(marker.line) || 1,
      endLineNumber: marker.line,
      endColumn: model.getLineMaxColumn(marker.line),
    })));
  }, [markers, value]);

  useEffect(() => {
    const view = editor.current;
    const model = view?.getModel();
    if (selectedLine === null || view === null || model === null || model === undefined) return;
    const line = Math.max(1, Math.min(selectedLine.line, model.getLineCount()));
    view.setSelection(new monaco.Selection(line, 1, line, model.getLineMaxColumn(line)));
    view.revealLineInCenter(line);
    view.focus();
    // The request, not the line, says a line was handed over again.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedLine?.request]);

  return (
    <div data-code-field className={`${codeFieldFrame} ${disabled ? "opacity-70" : ""} ${fill ? "h-full min-h-64" : ""}`}>
      <div ref={container} {...keyboardOwnerProps} style={fill ? undefined : { height }} className={fill ? "h-full w-full" : "w-full"} />
    </div>
  );
}
