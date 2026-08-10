# Navigation and Workflow Usability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make prerequisite detours resumable, distinguish ad hoc from connection diagnostics, and remove misleading no-op or ambiguously scoped actions.

**Architecture:** App owns a non-secret `CreateConnectionDraft` only while a connection workflow is paused. Existing section components remain independent; callbacks connect the modal to Keys and Groups, while a shell return bar resumes Connections. Other changes derive enabled state and copy from existing local state without backend changes.

**Tech Stack:** React 19, TypeScript, Vitest, Testing Library, Playwright, existing Go server and OpenAPI client.

## Global Constraints

- Do not add packages or change dependency manifests.
- Never retain passwords or vault passphrases outside `CreateConnectionModal`.
- Do not launch SSH, a terminal, reachability checks or authentication checks without an explicit action.
- Use existing English and Japanese message catalogs for every visible string.

---

### Task 1: Resumable connection prerequisites

**Files:**
- Modify: `web/src/connections/CreateConnectionModal.tsx`
- Modify: `web/src/connections/CreateConnectionModal.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Produces: exported `CreateConnectionDraft` containing alias, group, hostName, user, port, authentication kind, credential names and key ID, but no secret values.
- Produces: modal prerequisite callback `(section: "Keys" | "Groups", draft: CreateConnectionDraft) => void`.

- [ ] Add failing modal tests proving key-first default, required guidance, group/key prerequisite callbacks and absence of secrets from the handed-off draft.
- [ ] Run the focused modal test and verify failures describe the missing workflow.
- [ ] Implement draft initialisation, safe draft capture, prerequisite actions and key-first selection.
- [ ] Add failing Connections/App tests for storing a draft, navigating away, showing the return bar, resuming the modal and clearing the draft on cancel/success.
- [ ] Run the focused tests and verify those flows fail before integration.
- [ ] Wire the shell-owned draft and return bar through App and ConnectionsPage.
- [ ] Run the focused tests and typecheck, then commit.

### Task 2: Honest action availability and advanced file actions

**Files:**
- Modify: `web/src/connections/HostDetail.tsx`
- Modify: `web/src/connections/HostDetail.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Modify: `web/src/groups/GroupsPanel.tsx`
- Modify: `web/src/groups/GroupsPanel.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Consumes: existing draft/edit state in each component.
- Produces: disabled controls whenever their request would contain no change.

- [ ] Add failing tests for unchanged field/raw/comment/rename/group actions, empty file move and clean group preview/save.
- [ ] Run the focused tests and confirm each currently actionable no-op fails its assertion.
- [ ] Derive dirty state, disable the controls and rename Manage connection to Advanced file actions.
- [ ] Add immediate-write labels next to group rename/remove without changing backend semantics.
- [ ] Run focused tests and typecheck, then commit.

### Task 3: Distinct diagnostics and scoped navigation

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/overview/OverviewPanel.tsx`
- Modify: `web/src/overview/OverviewPanel.test.tsx`
- Modify: `web/src/diagnostics/DiagnosticsPanel.tsx`
- Modify: `web/src/diagnostics/DiagnosticsPanel.test.tsx`
- Modify: `web/src/ui/Inspector.tsx`
- Modify: `web/src/ui/Inspector.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Produces: `DiagnosticsPanel` optional `hosts: string[]` suggestions for standalone use.
- Produces: Home destinations `Config` and `History` based on the attention source.

- [ ] Add failing tests for Ad hoc checks labels, alias suggestions, Home's configuration destination and visible Details toggle.
- [ ] Run focused tests and verify the old labels/destinations fail.
- [ ] Pass aliases from App overview state, add the datalist, change Home routing and render the visible inspector label.
- [ ] Run focused tests and typecheck, then commit.

### Task 4: Notice and inventory naming clarity

**Files:**
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Modify: `web/src/keys/KeysScreen.test.tsx`
- Modify: `web/src/i18n/messages.ts`
- Modify: navigation label expectations in affected tests.

**Interfaces:**
- Produces: page-level notice filter that excludes detail/pattern codes.
- Produces: Classified SSH files and Install Key on Server labels.

- [ ] Add failing tests for scoped page notices and truthful section/metric labels.
- [ ] Run them to establish RED.
- [ ] Implement the filters and catalog changes.
- [ ] Run focused tests and typecheck, then commit.

### Task 5: End-to-end and regression verification

**Files:**
- Modify: `web/e2e/connections.spec.ts`
- Modify: affected E2E label selectors.
- Rebuild: `internal/ui/dist/`

**Interfaces:**
- Consumes: complete workflow from Tasks 1-4.
- Produces: browser evidence that a Keys detour resumes a non-secret draft and completes creation.

- [ ] Add the failing browser flow before rebuilding the embedded frontend.
- [ ] Build and confirm the old embedded UI fails the new E2E expectation.
- [ ] Rebuild the current frontend and run focused E2E.
- [ ] Run `make verify-generated`, `make build`, `make test` and full Playwright.
- [ ] Inspect dependency manifests, `git diff --check`, `git status --short`, commit the distribution build and push `main` only after every required check passes.
