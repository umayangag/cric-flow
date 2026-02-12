import React from 'react';
import { TableStat } from '../types';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Typography,
} from '@mui/material';

interface Props {
  stats?: TableStat[];
}

const OpsTableStats: React.FC<Props> = ({ stats }) => {
  if (!stats || stats.length === 0) {
    return null;
  }

  return (
    <TableContainer component={Paper} variant="outlined" sx={{ mt: 2 }}>
      <Table size="small" aria-label="database table statistics">
        <TableHead>
          <TableRow>
            <TableCell>Table Name</TableCell>
            <TableCell align="right">Row Count</TableCell>
            <TableCell align="right">Last Record</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {stats.map((row) => (
            <TableRow key={row.table_name}>
              <TableCell component="th" scope="row" sx={{ fontFamily: 'monospace' }}>
                {row.table_name}
              </TableCell>
              <TableCell align="right">{row.row_count.toLocaleString()}</TableCell>
              <TableCell align="right">
                {row.last_record || (
                  <Typography variant="caption" color="text.secondary">
                    -
                  </Typography>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

export default OpsTableStats;
