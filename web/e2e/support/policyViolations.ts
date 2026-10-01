import type { Page } from "@playwright/test";

// How Chromium reports a Content Security Policy violation: a refused load or
// a Trusted Types policy it may not create name the policy, and a string
// written where Trusted Types require a trusted value names the type it wanted
// ("This document requires 'TrustedHTML' assignment.").
const policyViolationReport = /Content Security Policy|Trusted ?Type|requires 'Trusted[A-Za-z]+' assignment/i;

// watchForPolicyViolations collects every Content Security Policy violation,
// Trusted Types included, that the page reports from now on. A blocked
// assignment is also thrown in the page, so page errors are collected too.
export function watchForPolicyViolations(page: Page): string[] {
  const violations: string[] = [];
  page.on("console", (message) => {
    const text = message.text();
    if (policyViolationReport.test(text)) violations.push(text);
  });
  page.on("pageerror", (error) => {
    if (policyViolationReport.test(error.message)) violations.push(error.message);
  });
  return violations;
}
