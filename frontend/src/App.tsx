import React, { useMemo, lazy, Suspense } from 'react';
import { Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { AuthProvider, useAuth } from './context/AuthContext';
import { MetricGlossaryProvider } from './context/MetricGlossaryContext';
import {
  AppBar,
  Box,
  Button,
  Chip,
  CircularProgress,
  Container,
  Fade,
  Paper,
  Tab,
  Tabs,
  Toolbar,
  Typography,
} from '@mui/material';

const HealthTab = lazy(() => import('./components/HealthTab'));
const EvaluationReportTab = lazy(() => import('./components/EvaluationReportTab'));
const OpsStatusTab = lazy(() => import('./components/OpsStatusTab'));
const UpcomingMatchTab = lazy(() => import('./components/UpcomingMatchTab'));
const WorkbenchTab = lazy(() => import('./components/WorkbenchTab'));
const SystemMapTab = lazy(() => import('./components/SystemMapTab'));
const Login = lazy(() => import('./pages/Login'));

const ProtectedRoute: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { isAuthenticated } = useAuth();
  const location = useLocation();

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  return <>{children}</>;
};

const AppContent: React.FC = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const { isAuthenticated, logout } = useAuth();
  const baseUrl = useMemo(() => import.meta.env.VITE_ML_SERVICE_URL || 'http://localhost:8000', []);

  // Determine active tab from path
  const currentTab = (() => {
    if (location.pathname.startsWith('/ops')) return 'ops';
    if (location.pathname.startsWith('/evaluate')) return 'evaluateDb';
    if (location.pathname.startsWith('/upcoming')) return 'upcoming';
    if (location.pathname.startsWith('/workbench')) return 'workbench';
    if (location.pathname.startsWith('/system-map')) return 'systemMap';
    return 'health';
  })();

  const handleChange = (_: React.SyntheticEvent, newValue: string) => {
    if (newValue === 'health') navigate('/health');
    else if (newValue === 'ops') navigate('/ops');
    else if (newValue === 'evaluateDb') navigate('/evaluate');
    else if (newValue === 'upcoming') navigate('/upcoming');
    else if (newValue === 'workbench') navigate('/workbench');
    else if (newValue === 'systemMap') navigate('/system-map');
  };

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        minHeight: '100vh',
        bgcolor: (t) => t.palette.background.default,
      }}
    >
      <AppBar
        position="static"
        elevation={0}
        sx={{
          bgcolor: 'background.paper',
          color: 'text.primary',
          borderBottom: '1px solid',
          borderColor: 'divider',
          '& .MuiToolbar-root': {
            minHeight: { xs: 56, sm: 64 },
            px: { xs: 2, sm: 3 },
          },
        }}
      >
        <Toolbar
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: 2,
          }}
        >
          <Typography
            variant="h6"
            component="div"
            sx={{
              fontWeight: 600,
              letterSpacing: '-0.02em',
              color: 'text.primary',
            }}
          >
            Cric Info — ML Control Panel
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, minWidth: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
              <Chip
                label="ML Service"
                size="small"
                sx={{
                  bgcolor: 'rgba(14, 165, 233, 0.12)',
                  color: 'primary.dark',
                  fontWeight: 500,
                  border: 'none',
                  '& .MuiChip-label': { px: 1.25 },
                }}
              />
              <Typography
                variant="body2"
                sx={{
                  color: 'text.secondary',
                  display: { xs: 'none', sm: 'inline' },
                  fontFamily: 'monospace',
                  fontSize: '0.8rem',
                }}
                noWrap
              >
                {baseUrl}
              </Typography>
            </Box>
            {isAuthenticated && (
              <>
                <Box
                  sx={{
                    width: '1px',
                    height: 20,
                    bgcolor: 'divider',
                    display: { xs: 'none', sm: 'block' },
                  }}
                  aria-hidden
                />
                <Button
                  color="inherit"
                  onClick={logout}
                  size="small"
                  sx={{
                    textTransform: 'none',
                    fontWeight: 500,
                    color: 'text.primary',
                    '&:hover': { bgcolor: 'action.hover' },
                  }}
                >
                  Logout
                </Button>
              </>
            )}
          </Box>
        </Toolbar>
      </AppBar>

      <Container maxWidth="lg" sx={{ my: 3, flexGrow: 1, width: '100%' }}>
        {/* Tabs - Only show if authenticated */}
        {isAuthenticated && (
          <Tabs
            value={currentTab}
            onChange={handleChange}
            variant="scrollable"
            scrollButtons="auto"
            aria-label="Main sections"
            sx={{ mb: 2 }}
          >
            <Tab value="health" label="Health" />
            <Tab value="ops" label="Ops Status" />
            <Tab value="workbench" label="Workbench" />
            <Tab value="evaluateDb" label="Evaluation report" />
            <Tab value="upcoming" label="Upcoming match prediction" />
            <Tab value="systemMap" label="System map" />
          </Tabs>
        )}

        {/* Content Card. The metric glossary (L-1) is fetched once here and read by every
            surface that shows a number; it needs the API key, so it waits for the login. */}
        <Fade in timeout={240}>
          <Paper elevation={2} sx={{ p: 2, borderRadius: 2 }}>
            <Suspense
              fallback={
                <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                  <CircularProgress />
                </Box>
              }
            >
              <MetricGlossaryProvider enabled={isAuthenticated}>
                <Routes>
                  <Route path="/" element={<Navigate to="/health" replace />} />
                  <Route path="/login" element={<Login />} />
                  <Route
                    path="/health"
                    element={
                      <ProtectedRoute>
                        <HealthTab />
                      </ProtectedRoute>
                    }
                  />
                  <Route
                    path="/ops"
                    element={
                      <ProtectedRoute>
                        <OpsStatusTab />
                      </ProtectedRoute>
                    }
                  />
                  <Route
                    path="/evaluate"
                    element={
                      <ProtectedRoute>
                        <EvaluationReportTab />
                      </ProtectedRoute>
                    }
                  />
                  <Route
                    path="/upcoming"
                    element={
                      <ProtectedRoute>
                        <UpcomingMatchTab />
                      </ProtectedRoute>
                    }
                  />
                  <Route
                    path="/workbench"
                    element={
                      <ProtectedRoute>
                        <WorkbenchTab />
                      </ProtectedRoute>
                    }
                  />
                  <Route
                    path="/system-map"
                    element={
                      <ProtectedRoute>
                        <SystemMapTab />
                      </ProtectedRoute>
                    }
                  />
                  <Route path="*" element={<Navigate to="/health" replace />} />
                </Routes>
              </MetricGlossaryProvider>
            </Suspense>
          </Paper>
        </Fade>
      </Container>
    </Box>
  );
};

const App: React.FC = () => {
  return (
    <AuthProvider>
      <AppContent />
    </AuthProvider>
  );
};

export default App;
