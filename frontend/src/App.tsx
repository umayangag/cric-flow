import React, { useMemo, useState } from 'react';
import HealthTab from './components/HealthTab';
import PredictTab from './components/PredictTab';
import EvaluateTab from './components/EvaluateTab';

type TabKey = 'health' | 'predict' | 'evaluate';

const TabButton: React.FC<{ id: TabKey; active: boolean; onClick: (id: TabKey) => void }> = ({ id, active, onClick, children }) => (
  <button
    onClick={() => onClick(id)}
    style={{
      padding: '8px 12px',
      border: '1px solid #ccc',
      background: active ? '#eef' : '#fff',
      cursor: 'pointer',
      borderBottom: active ? '2px solid #55f' : '1px solid #ccc',
      marginRight: 8,
    }}
  >
    {children}
  </button>
);

const App: React.FC = () => {
  const [tab, setTab] = useState<TabKey>('health');
  const baseUrl = useMemo(() => (import.meta.env.VITE_ML_SERVICE_URL as string) || 'http://localhost:8000', []);

  return (
    <div style={{ fontFamily: 'system-ui, Arial, sans-serif', padding: 16 }}>
      <h1 style={{ marginTop: 0 }}>ML Control Panel</h1>
      <div style={{ marginBottom: 12 }}>
        <small>ML Service: {baseUrl}</small>
      </div>
      <div style={{ marginBottom: 16 }}>
        <TabButton id="health" active={tab === 'health'} onClick={setTab}>
          Health
        </TabButton>
        <TabButton id="predict" active={tab === 'predict'} onClick={setTab}>
          Single Prediction
        </TabButton>
        <TabButton id="evaluate" active={tab === 'evaluate'} onClick={setTab}>
          Evaluate From CSV
        </TabButton>
      </div>

      {tab === 'health' && <HealthTab />}
      {tab === 'predict' && <PredictTab />}
      {tab === 'evaluate' && <EvaluateTab />}
    </div>
  );
};

export default App;
