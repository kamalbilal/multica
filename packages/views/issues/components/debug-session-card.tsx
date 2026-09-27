"use client";

import { useState, memo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Bug, ChevronRight } from "lucide-react";
import { issueDebugSessionOptions } from "@multica/core/issues/queries";
import { useCloseIssueDebugSession, useContinueIssueDebugSession } from "@multica/core/issues/mutations";
import { Button } from "@multica/ui/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@multica/ui/components/ui/collapsible";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

function hypothesisLabel(item: unknown, index: number): string {
  if (typeof item === "string") return item;
  if (item && typeof item === "object") {
    const row = item as { id?: unknown; text?: unknown; title?: unknown };
    const id = typeof row.id === "string" ? row.id : `H${index + 1}`;
    const text = typeof row.text === "string" ? row.text : typeof row.title === "string" ? row.title : "";
    return text ? `${id}: ${text}` : id;
  }
  return `H${index + 1}`;
}

export const DebugSessionCard = memo(function DebugSessionCard({ issueId }: { issueId: string }) {
  const { t } = useT("issues");
  const { data } = useQuery(issueDebugSessionOptions(issueId));
  const continueSession = useContinueIssueDebugSession(issueId);
  const closeSession = useCloseIssueDebugSession(issueId);
  const [note, setNote] = useState("");
  const [detailsOpen, setDetailsOpen] = useState(true);
  const session = data?.session;
  const waiting = session?.status === "waiting_repro" || session?.status === "waiting_verify";
  const analyzing = session?.status === "analyzing" || session?.status === "instrumenting";
  if (!session || (!waiting && !analyzing)) {
    return null;
  }

  const busy = continueSession.isPending || closeSession.isPending;
  const run = (action: "reproduced" | "comment" | "fixed", content?: string) => {
    continueSession.mutate({ action, content });
    if (action === "comment") setNote("");
  };

  const hasDetails =
    Boolean(session.work_dir_hint) ||
    (waiting && Boolean(session.repro_steps)) ||
    (waiting && session.hypotheses.length > 0);

  const title =
    session.status === "waiting_verify"
      ? t(($) => $.comment.debug.waiting_verify_title)
      : session.status === "waiting_repro"
        ? t(($) => $.comment.debug.waiting_repro_title)
        : t(($) => $.comment.debug.analyzing_title);

  return (
    <div className="mb-3 rounded-lg bg-card p-3 ring-1 ring-border">
      <Collapsible open={hasDetails ? detailsOpen : true} onOpenChange={setDetailsOpen}>
        <div className="mb-2 flex items-center gap-2 text-body font-medium">
          <Bug className="size-4 shrink-0" aria-hidden />
          {hasDetails ? (
            <CollapsibleTrigger
              className="flex min-w-0 flex-1 items-center gap-1 rounded-xs text-left hover:text-foreground"
              aria-label={
                detailsOpen
                  ? t(($) => $.comment.debug.details_collapse)
                  : t(($) => $.comment.debug.details_expand)
              }
            >
              <span className="min-w-0 flex-1">{title}</span>
              <ChevronRight
                className={cn(
                  "size-4 shrink-0 text-muted-foreground transition-transform motion-reduce:transition-none",
                  detailsOpen && "rotate-90",
                )}
                aria-hidden
              />
            </CollapsibleTrigger>
          ) : (
            <span className="min-w-0 flex-1">{title}</span>
          )}
          <span className="shrink-0 text-caption font-normal text-muted-foreground tabular-nums">
            {t(($) => $.comment.debug.logs_captured, { count: session.event_count ?? 0 })}
          </span>
        </div>
        {hasDetails ? (
          <CollapsibleContent>
            {session.work_dir_hint ? (
              <p className="mb-2 text-caption text-muted-foreground">
                {t(($) => $.comment.debug.work_dir_hint, { path: session.work_dir_hint })}
              </p>
            ) : null}
            {waiting && session.repro_steps ? (
              <p className="mb-2 whitespace-pre-wrap text-caption text-muted-foreground">
                {session.repro_steps}
              </p>
            ) : null}
            {waiting && session.hypotheses.length > 0 ? (
              <ul className="mb-3 list-disc space-y-1 pl-5 text-caption">
                {session.hypotheses.map((item, index) => (
                  <li key={index}>{hypothesisLabel(item, index)}</li>
                ))}
              </ul>
            ) : null}
          </CollapsibleContent>
        ) : null}
      </Collapsible>
      {waiting ? (
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" disabled={busy} onClick={() => run("reproduced")}>
            {t(($) => $.comment.debug.approved)}
          </Button>
          <Button size="sm" variant="outline" disabled={busy} onClick={() => run("fixed")}>
            {t(($) => $.comment.debug.looks_fixed)}
          </Button>
          <Button size="sm" variant="ghost" disabled={busy} onClick={() => closeSession.mutate()}>
            {t(($) => $.comment.debug.dismiss)}
          </Button>
        </div>
      ) : (
        <Button size="sm" variant="ghost" disabled={busy} onClick={() => closeSession.mutate()}>
          {t(($) => $.comment.debug.dismiss)}
        </Button>
      )}
      {waiting ? (
        <div className="mt-2 flex gap-2">
          <input
            className="min-w-0 flex-1 rounded-md border border-border bg-background px-2 py-1 text-caption"
            value={note}
            disabled={busy}
            placeholder={t(($) => $.comment.debug.comment_placeholder)}
            onChange={(e) => setNote(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && note.trim()) run("comment", note.trim());
            }}
          />
          <Button
            size="sm"
            variant="ghost"
            disabled={busy || !note.trim()}
            onClick={() => run("comment", note.trim())}
          >
            {t(($) => $.comment.debug.send_comment)}
          </Button>
        </div>
      ) : null}
    </div>
  );
});
