import styled from '@emotion/styled';

export const TableContainer = styled.div`
  overflow-x: auto;
  border: 1px solid #333;
  border-radius: 4px;
`;

export const Table = styled.table`
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
  text-align: left;
`;

export const TableHead = styled.thead`
  background: #222;
  color: #ccc;
`;

export const TableHeaderCell = styled.th`
  padding: 8px;
`;

export const TableRow = styled.tr`
  border-top: 1px solid #333;
`;

export const TableCell = styled.td`
  padding: 8px;
`;

export const CodeCell = styled(TableCell)`
  font-family: monospace;
`;

export const ErrorMessage = styled.div`
  color: red;
  max-width: 300px;
`;

export const ErrorText = styled.div`
  color: red;
`;

export const EmptyStateCell = styled(TableCell)`
  padding: 16px;
  text-align: center;
  color: #666;
`;
