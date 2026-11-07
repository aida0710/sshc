import { useEffect, type RefObject } from "react";
import type * as monaco from "monaco-editor/editor/editor.api.js";

// useModelFollowsValue writes a `value` the parent set itself, such as the contents read again from the engine, into
// the editor's model. reports holds the contents the editor reported through onChange that have not come back as
// `value` yet, oldest first: React can render one of them after the person has typed more, so `value` is not always
// the model's latest contents. A `value` the editor reported is skipped even when it differs from the model, as
// writing that older report would undo the typing since and move the cursor to the start.
export function useModelFollowsValue(
  editor: RefObject<monaco.editor.IStandaloneCodeEditor | null>,
  value: string,
  reports: RefObject<string[]>,
): void {
  useEffect(() => {
    const model = editor.current?.getModel();
    if (model === null || model === undefined) return;
    const report = reports.current.indexOf(value);
    if (report !== -1) {
      reports.current.splice(0, report + 1);
      return;
    }
    if (model.getValue() !== value) model.setValue(value);
    // Also drops the report of the setValue above: the parent set that value.
    reports.current = [];
  }, [editor, reports, value]);
}
