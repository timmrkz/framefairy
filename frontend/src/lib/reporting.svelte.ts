// Whether Report a Problem is open, as the Go side says, see bugs.go.
// There is one report at a time, so while it is open Bugs in the sidebar
// is lit and every Report button is greyed out. App.svelte listens from
// the start.

import { api, onReportWindow } from "./api";

class Reporting {
  open = $state(false);

  // Hears the window open and close, and asks where it stands now.
  listen(): () => void {
    const off = onReportWindow((open) => (this.open = open));
    api
      .reportWindowOpen()
      .then((open) => (this.open = open))
      .catch(() => {});
    return off;
  }
}

export const reporting = new Reporting();

// What a Report button says on hover, which says why it is greyed out
// while a report is being written.
export function reportTitle(): string {
  return reporting.open
    ? "Report a Problem is open already. Finish that report first"
    : "Opens Report a Problem with this message written in it";
}
