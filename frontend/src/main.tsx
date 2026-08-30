import React from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import { CssBaseline, ThemeProvider } from '@mui/material';
import RootErrorBoundary from './components/common/RootErrorBoundary';
import theme from './theme';

const container = document.getElementById('root')!;
const root = createRoot(container);

root.render(
  <RootErrorBoundary>
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ThemeProvider>
  </RootErrorBoundary>,
);

// Tells the bootstrap guard in index.html that startup succeeded, so a later
// runtime error does not replace the mounted app with the startup panel.
window.__cricFlowMounted = true;
