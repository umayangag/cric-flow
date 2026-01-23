import React, { useMemo, useState } from 'react';
import HealthTab from './components/HealthTab';
import PredictTab from './components/PredictTab';
import EvaluateTab from './components/EvaluateTab';
import EvaluateDbTab from './components/EvaluateDbTab';
import MatchCompareDbTab from './components/MatchCompareDbTab';
import OpsStatusTab from './components/OpsStatusTab';
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

type TabKey = 'health' | 'predict' | 'evaluate' | 'evaluateDb' | 'compareDb' | 'ops';

const App: React.FC = () => {
  const [tab, setTab] = useState<TabKey>('health');
  const baseUrl = useMemo(() => import.meta.env.VITE_ML_SERVICE_URL || 'http://localhost:8000', []);

  const handleChange = (_: React.SyntheticEvent, newValue: TabKey) => {
    setTab(newValue);
  };

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', minHeight: '100vh', bgcolor: (t) => t.palette.background.default }}>
      {/* AppBar with subtle gradient */}
      <AppBar
        position="static"
        sx={{
          background: (theme) => `linear-gradient(90deg, ${theme.palette.primary.main}, ${theme.palette.primary.dark})`,
          boxShadow: 2,
        }}
      >
        <Toolbar sx={{ display: 'flex', justifyContent: 'space-between' }}>
          <Typography variant="h6" component="div">
            Cric Info — ML Control Panel
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
            <Chip color="secondary" label="ML Service" size="small" />
            <Typography variant="body2" sx={{ opacity: 0.8, display: { xs: 'none', sm: 'inline' } }} noWrap>
              {baseUrl}
            </Typography>
          </Box>
        </Toolbar>
      </AppBar>

      <Container maxWidth="lg" sx={{ my: 3, flexGrow: 1, width: '100%' }}>
        {/* Tabs */}
        <Tabs
          value={tab}
          onChange={handleChange}
          variant="scrollable"
          scrollButtons="auto"
          aria-label="Main sections"
          sx={{ mb: 2 }}
        >
          <Tab value="health" label="Health" />
          <Tab value="ops" label="Ops Status" />
          <Tab value="predict" label="Single Prediction" />
          <Tab value="evaluate" label="Evaluate From CSV" />
          <Tab value="evaluateDb" label="Evaluate (DB)" />
          <Tab value="compareDb" label="Match Compare (DB)" />
        </Tabs>

        {/* Content Card */}
        <Fade in timeout={240}>
          <Paper elevation={2} sx={{ p: 2 }}>
            {tab === 'health' && <HealthTab />}
            {tab === 'ops' && <OpsStatusTab />}
            {tab === 'predict' && <PredictTab />}
            {tab === 'evaluate' && <EvaluateTab />}
            {tab === 'evaluateDb' && <EvaluateDbTab />}
            {tab === 'compareDb' && <MatchCompareDbTab />}
          </Paper>
        </Fade>
      </Container>
    </Box>
  );
};

export default App;
