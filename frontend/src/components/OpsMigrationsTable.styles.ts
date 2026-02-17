import styled from '@emotion/styled';

const borderSoft = '1px solid rgba(0, 0, 0, 0.08)';
const borderSofter = '1px solid rgba(0, 0, 0, 0.06)';
const headerBg = '#f8fafc';
const rowHoverBg = 'rgba(0, 0, 0, 0.02)';

export const TableContainer = styled.div`
  overflow-x: auto;
  border: ${borderSoft};
  border-radius: 12px;
  background: #fff;
`;

export const Table = styled.table`
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
  text-align: left;
`;

export const TableHead = styled.thead`
  background: ${headerBg};
`;

export const TableHeaderCell = styled.th`
  padding: 12px 14px;
  font-weight: 600;
  color: #475569;
  border-bottom: ${borderSoft};
  &:not(:last-child) {
    border-right: ${borderSofter};
  }
`;

export const TableRow = styled.tr`
  border-bottom: ${borderSofter};
  transition: background-color 0.15s ease;
  &:last-child {
    border-bottom: none;
  }
  &:hover {
    background: ${rowHoverBg};
  }
`;

export const TableCell = styled.td`
  padding: 12px 14px;
  color: #334155;
  &:not(:last-child) {
    border-right: ${borderSofter};
  }
`;

export const CodeCell = styled(TableCell)`
  font-family: ui-monospace, Menlo, monospace;
  font-size: 12px;
  color: #475569;
`;

export const ErrorMessage = styled.div`
  color: #dc2626;
  max-width: 320px;
  font-size: 12px;
  line-height: 1.4;
`;

export const ErrorText = styled.div`
  color: #dc2626;
`;

export const EmptyStateCell = styled(TableCell)`
  padding: 24px 16px;
  text-align: center;
  color: #64748b;
  font-size: 14px;
`;

export const PaginationContainer = styled.div`
  display: flex;
  justify-content: flex-end;
  align-items: center;
  padding: 10px 14px;
  gap: 10px;
  background: ${headerBg};
  border-top: ${borderSoft};
  border-radius: 0 0 12px 12px;
  font-size: 13px;
  color: #64748b;
`;

export const PaginationButton = styled.button`
  background: #fff;
  color: #475569;
  border: ${borderSoft};
  padding: 6px 14px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease,
    color 0.2s ease;

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  &:hover:not(:disabled) {
    background: rgba(0, 0, 0, 0.04);
    border-color: rgba(0, 0, 0, 0.12);
  }
`;
