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
          background: (t) =>
            `linear-gradient(90deg, ${t.palette.primary.main}, ${t.palette.primary.dark})`,
          boxShadow: '0px 1px 3px rgba(0, 0, 0, 0.08)',
        }}
      >
        <Toolbar sx={{ display: 'flex', justifyContent: 'space-between' }}>
          <Typography variant="h6" component="div">
            Cric Info — ML Control Panel
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, minWidth: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Chip color="secondary" label="ML Service" size="small" />
              <Typography
                variant="body2"
                sx={{ opacity: 0.8, display: { xs: 'none', sm: 'inline' } }}
                noWrap
              >
                {baseUrl}
              </Typography>
            </Box>
            {isAuthenticated && (
              <Button color="inherit" onClick={logout} size="small">
                Logout
              </Button>
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
