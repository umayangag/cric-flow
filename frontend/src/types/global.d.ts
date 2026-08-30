declare global {
  interface Window {
    /** Set by main.tsx once React has mounted; read by the bootstrap guard in index.html. */
    __cricFlowMounted?: boolean;
  }
}

export {};
