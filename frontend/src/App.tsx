import React, { useMemo, useState } from 'react';
import HealthTab from './components/HealthTab';
import PredictTab from './components/PredictTab';
import EvaluateTab from './components/EvaluateTab';
import EvaluateDbTab from './components/EvaluateDbTab';
import MatchCompareDbTab from './components/MatchCompareDbTab';

type TabKey = 'health' | 'predict' | 'evaluate' | 'evaluateDb' | 'compareDb';

type TabButtonProps = {
  id: TabKey;
  active: boolean;
  onClick: React.Dispatch<React.SetStateAction<TabKey>>;
  children?: React.ReactNode;
};

const styles = {
  container: {
    fontFamily: 'system-ui, Arial, sans-serif',
    padding: 16,
  } as React.CSSProperties,
  title: {
    marginTop: 0,
  } as React.CSSProperties,
  info: {
    marginBottom: 12,
  } as React.CSSProperties,
  tabs: {
    marginBottom: 16,
  } as React.CSSProperties,
  tabButton: {
    padding: '8px 12px',
    border: '1px solid #ccc',
    background: '#fff',
    cursor: 'pointer',
    borderBottom: '1px solid #ccc',
    marginRight: 8,
  } as React.CSSProperties,
  tabButtonActive: {
    background: '#eef',
    borderBottom: '2px solid #55f',
  } as React.CSSProperties,
};

const TabButton: React.FC<TabButtonProps> = ({ id, active, onClick, children }) => (
  <button
    onClick={() => onClick(id)}
    style={active ? { ...styles.tabButton, ...styles.tabButtonActive } : styles.tabButton}
  >
    {children}
  </button>
);

const App: React.FC = () => {
  const [tab, setTab] = useState<TabKey>('health');
  const baseUrl = useMemo(() => import.meta.env.VITE_ML_SERVICE_URL || 'http://localhost:8000', []);

  return (
    <div style={styles.container}>
      <h1 style={styles.title}>ML Control Panel</h1>
      <div style={styles.info}>
        <small>ML Service: {baseUrl}</small>
      </div>
      <div style={styles.tabs}>
        <TabButton id="health" active={tab === 'health'} onClick={setTab}>
          Health
        </TabButton>
        <TabButton id="predict" active={tab === 'predict'} onClick={setTab}>
          Single Prediction
        </TabButton>
        <TabButton id="evaluate" active={tab === 'evaluate'} onClick={setTab}>
          Evaluate From CSV
        </TabButton>
        <TabButton id="evaluateDb" active={tab === 'evaluateDb'} onClick={setTab}>
          Evaluate (DB)
        </TabButton>
        <TabButton id="compareDb" active={tab === 'compareDb'} onClick={setTab}>
          Match Compare (DB)
        </TabButton>
      </div>

      {tab === 'health' && <HealthTab />}
      {tab === 'predict' && <PredictTab />}
      {tab === 'evaluate' && <EvaluateTab />}
      {tab === 'evaluateDb' && <EvaluateDbTab />}
      {tab === 'compareDb' && <MatchCompareDbTab />}
    </div>
  );
};

export default App;
