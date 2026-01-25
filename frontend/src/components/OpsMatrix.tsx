import React from "react";

type MatrixType = "precompute" | "exports" | "artifacts";

type Props = {
  type: MatrixType;
  title: string;
  data: any;
};

const FORMATS = ["TEST", "ODI", "T20I", "T20"] as const;

const cellStyle = (bg: string, title?: string): React.CSSProperties => ({
  background: bg,
  color: "#eee",
  borderRadius: 6,
  padding: "8px 10px",
  fontSize: 12,
  minWidth: 90,
  textAlign: "center",
  border: "1px solid rgba(255,255,255,0.06)",
});

const legendDot = (color: string): JSX.Element => (
  <span
    style={{
      display: "inline-block",
      width: 8,
      height: 8,
      borderRadius: 999,
      background: color,
      marginRight: 6,
    }}
  />
);

export const OpsMatrix: React.FC<Props> = ({ type, title, data }) => {
  const renderPrecompute = () => (
    <div style={{ display: "grid", gap: 8 }}>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {FORMATS.map((f) => {
          const st = data?.formats?.[f]?.status as string | undefined;
          const color =
            st === "ok"
              ? "#17431d"
              : st === "stale"
                ? "#54450f"
                : st === "missing"
                  ? "#4a1010"
                  : "#333";
          const txt = st ?? "unknown";
          return (
            <div
              key={f}
              style={cellStyle(color)}
              title={`status: ${txt}`}
              data-testid={`precompute-${f}`}
            >
              <strong>{f}</strong>
              <div style={{ fontSize: 11, opacity: 0.9 }}>{txt}</div>
            </div>
          );
        })}
      </div>
      <div style={{ fontSize: 11, opacity: 0.8 }}>
        {legendDot("#17431d")} ok {legendDot("#54450f")} stale{" "}
        {legendDot("#4a1010")} missing
      </div>
    </div>
  );

  const hasAnyExists = (files: any): boolean => {
    if (Array.isArray(files)) {
      // files could be []map or []any; normalize
      return files.some(
        (e: any) => !!(e && typeof e === "object" && e.exists === true),
      );
    }
    return false;
  };

  const renderExports = () => (
    <div style={{ display: "grid", gap: 8 }}>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {FORMATS.map((f) => {
          const files = data?.formats?.[f]?.files ?? [];
          const ok = hasAnyExists(files);
          const color = ok ? "#17431d" : "#4a1010";
          const titleStr = Array.isArray(files)
            ? files
                .map((x: any) => x?.name)
                .filter(Boolean)
                .join(", ")
            : "";
          return (
            <div
              key={f}
              style={cellStyle(color)}
              title={titleStr}
              data-testid={`exports-${f}`}
            >
              <strong>{f}</strong>
              <div style={{ fontSize: 11, opacity: 0.9 }}>
                {ok ? "present" : "missing"}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );

  const subCell = (ok: boolean, loaded?: boolean) => (
    <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
      <span
        style={{
          display: "inline-block",
          width: 10,
          height: 10,
          borderRadius: 2,
          background: ok ? "#17431d" : "#4a1010",
          border: "1px solid rgba(255,255,255,0.15)",
        }}
      />
      <span style={{ fontSize: 11, opacity: 0.9 }}>
        {ok ? "exists" : "missing"}
      </span>
      {loaded ? (
        <span
          title="loaded"
          style={{
            width: 7,
            height: 7,
            background: "#48d597",
            borderRadius: 999,
          }}
        />
      ) : null}
    </div>
  );

  const renderArtifacts = () => (
    <div style={{ display: "grid", gap: 8 }}>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {FORMATS.map((f) => {
          const bat = data?.formats?.[f]?.batting ?? {};
          const bowl = data?.formats?.[f]?.bowling ?? {};
          const ok = bat?.exists === true && bowl?.exists === true;
          const color = ok ? "#17431d" : "#4a1010";
          return (
            <div
              key={f}
              style={cellStyle(color)}
              data-testid={`artifacts-${f}`}
            >
              <strong>{f}</strong>
              <div style={{ display: "grid", gap: 6, marginTop: 6 }}>
                <div>
                  {subCell(bat?.exists === true, bat?.loaded === true)}{" "}
                  <small>batting</small>
                </div>
                <div>
                  {subCell(bowl?.exists === true, bowl?.loaded === true)}{" "}
                  <small>bowling</small>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );

  return (
    <section>
      <h3 style={{ margin: "8px 0" }}>{title}</h3>
      {type === "precompute" && renderPrecompute()}
      {type === "exports" && renderExports()}
      {type === "artifacts" && renderArtifacts()}
    </section>
  );
};

export default OpsMatrix;
