import React, { useMemo } from 'react';
import { Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom';
import HealthTab from './components/HealthTab';
import EvaluateDbTab from './components/EvaluateDbTab';
import OpsStatusTab from './components/OpsStatusTab';
import Login from './pages/Login';
import { AuthProvider, useAuth } from './context/AuthContext';
import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';
import Container from '@mui/material/Container';
import Tabs from '@mui/material/Tabs';
import Tab from '@mui/material/Tab';
import Paper from '@mui/material/Paper';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Fade from '@mui/material/Fade';
import Button from '@mui/material/Button';

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
    return 'health';
  })();

  const handleChange = (_: React.SyntheticEvent, newValue: string) => {
    if (newValue === 'health') navigate('/health');
    else if (newValue === 'ops') navigate('/ops');
    else if (newValue === 'evaluateDb') navigate('/evaluate');
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
            <Tab value="evaluateDb" label="Evaluate (DB)" />
          </Tabs>
        )}

        {/* Content Card */}
        <Fade in timeout={240}>
          <Paper elevation={2} sx={{ p: 2, borderRadius: 2 }}>
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
                    <EvaluateDbTab />
                  </ProtectedRoute>
                }
              />
              <Route path="*" element={<Navigate to="/health" replace />} />
            </Routes>
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
