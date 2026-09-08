import React from 'react';
import {
  Alert,
  Box,
  Button,
  Chip,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import ErrorNotice from './common/ErrorNotice';
import { MetricLabel } from './common/MetricInfo';
import RatingsAsOf from './RatingsAsOf';
import type {
  AuctionIntervalSource,
  AuctionListedPlayer,
  AuctionProjection,
  AuctionProjectionToss,
  AuctionQuantiles,
} from '../types';
import type { ApiError } from '../lib/apiError';

/**
 * A candidate's projected output, per ground (P3-2).
 *
 * **The rule this surface is built on.** Valuation and projection, never XI-picking. There
 * is no win probability here and no marginal value — go-app's own type for `/simulate`
 * carries no such field, so neither can reach this component — and no request from it
 * reaches the optimiser. In T20 the system has not shown it can choose an eleven better
 * than rating order (plan §8.8), and the IPL is domestic T20.
 *
 * **Every interval is the stack's, as served.** Two of them sit side by side on each row
 * and they are different populations: L2-B's per-player quantile heads, at nominal
 * coverage on the harness, and the simulator's drawn totals, which are B-11's open defect.
 * Each names its source and opens its own explainer. Nothing is widened, narrowed, adjusted
 * for day or night, or hidden — the ranges rule P1-4 set for the Lab.
 *
 * **Nothing here is computed in the browser.** The per-ground numbers and the mixture all
 * arrive on the answer; a range recomputed here would be a second definition of an
 * interval this surface exists to report.
 */

export type AuctionProjectionPanelProps = {
  listed: AuctionListedPlayer[];
  projection: AuctionProjection | null;
  projecting: boolean;
  error: ApiError | null;
  onProject: (request: {
    playerId: number;
    team1BatsFirst?: boolean;
    venueWeights?: { venue_id: number; weight: number }[];
  }) => void;
};

/** How the toss reads on screen, in the buyer's terms rather than the wire's. */
const TOSS_LABELS: Record<AuctionProjectionToss, string> = {
  unknown: 'Toss unknown — both batting orders averaged',
  candidate_eleven_bats_first: 'His eleven bats first',
  candidate_eleven_chases: 'His eleven chases',
};

/** One number with its 10-90 band, and the source of that band named beside it. */
const Quantiles: React.FC<{ quantiles: AuctionQuantiles }> = ({ quantiles }) => (
  <>
    <Typography variant="body2" component="span">
      {quantiles.median.toFixed(1)}
    </Typography>
    <Typography variant="caption" color="text.secondary" component="div">
      {quantiles.q10.toFixed(1)}–{quantiles.q90.toFixed(1)}
    </Typography>
  </>
);

/**
 * An interval source's label with its explainer.
 *
 * The L-1 key is spelled here as a literal per source rather than read off the wire (H-24):
 * a source this surface could not name is then a failing contract test, and not an
 * interval rendered with no explainer behind it.
 */
const IntervalSourceLabel: React.FC<{ source: AuctionIntervalSource; label: string }> = ({
  source,
  label,
}) =>
  source === 'l2b_quantiles' ? (
    <MetricLabel metricKey="interval_source_l2b_quantiles" label={label} />
  ) : (
    <MetricLabel metricKey="interval_source_simulator_draws" label={label} />
  );

/** The two interval sources, each with what it is and what is open against it. */
const IntervalSources: React.FC<{ projection: AuctionProjection }> = ({ projection }) => (
  <Box sx={{ mt: 2 }} data-testid="auction-interval-sources">
    <Typography variant="subtitle2">Where each range comes from</Typography>
    <Stack spacing={1} sx={{ mt: 0.5 }}>
      {projection.intervals.map((interval) => (
        <Box key={interval.source} data-testid={`auction-interval-${interval.source}`}>
          <Typography variant="body2" fontWeight={600} component="div">
            <IntervalSourceLabel source={interval.source} label={interval.label} />
          </Typography>
          <Typography variant="caption" color="text.secondary" component="div">
            {interval.note}
          </Typography>
          {interval.caveats && interval.caveats.length > 0 && (
            <Stack direction="row" spacing={0.5} sx={{ mt: 0.5 }}>
              {interval.caveats.map((caveat) => (
                <Chip key={caveat} size="small" color="warning" variant="outlined" label={caveat} />
              ))}
            </Stack>
          )}
        </Box>
      ))}
    </Stack>
  </Box>
);

/** The assumptions the answer was made under, carried back and shown as assumptions. */
const Assumptions: React.FC<{ projection: AuctionProjection }> = ({ projection }) => (
  <Box sx={{ mt: 1 }} data-testid="auction-projection-assumptions">
    <Typography variant="body2" data-testid="auction-projection-conditional">
      For <strong>{projection.candidate.player_name}</strong> in this eleven (
      {projection.assumptions.eleven.map((player) => player.player_name).join(', ')}), against{' '}
      <strong>{projection.assumptions.opposition.name}</strong>’s named eleven, at{' '}
      {projection.assumptions.grounds.map((ground) => ground.venue_name).join(', ')}.
    </Typography>
    <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }}>
      {TOSS_LABELS[projection.assumptions.toss]}.
    </Typography>
    <Alert severity="info" sx={{ mt: 1 }} data-testid="auction-what-a-ground-changes">
      {projection.assumptions.what_a_ground_changes}
    </Alert>
    <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
      {projection.assumptions.not_xi_picking}
    </Typography>
  </Box>
);

/** One row per ground, side by side: the venue mix, as the model reads a venue. */
const GroundRows: React.FC<{ projection: AuctionProjection }> = ({ projection }) => (
  <Table size="small" sx={{ mt: 2 }} data-testid="auction-ground-rows">
    <TableHead>
      <TableRow>
        <TableCell>Ground</TableCell>
        <TableCell align="right">
          <MetricLabel metricKey="auction_projected_output" label="Runs" />
        </TableCell>
        <TableCell align="right">Balls faced</TableCell>
        <TableCell align="right">Runs conceded</TableCell>
        <TableCell align="right">Wickets</TableCell>
        <TableCell align="right">
          <MetricLabel metricKey="auction_projected_total" label="The eleven’s total" />
        </TableCell>
        <TableCell align="right">
          <MetricLabel metricKey="spread_share" label="His share of its spread" />
        </TableCell>
      </TableRow>
    </TableHead>
    <TableBody>
      {projection.grounds.map((ground) => (
        <TableRow key={ground.venue_id} data-testid={`auction-ground-${ground.venue_id}`}>
          <TableCell>
            {ground.venue_name || `venue ${ground.venue_id}`}
            <Typography variant="caption" color="text.secondary" component="div">
              {ground.ground.neutral
                ? 'reads neutral'
                : `bat-first ${ground.ground.bat_first_rate.toFixed(2)} over ${ground.ground.matches} matches`}
            </Typography>
          </TableCell>
          <TableCell align="right">
            <Quantiles quantiles={ground.candidate.runs} />
          </TableCell>
          <TableCell align="right">
            <Quantiles quantiles={ground.candidate.balls_faced} />
          </TableCell>
          <TableCell align="right">
            <Quantiles quantiles={ground.candidate.runs_conceded} />
          </TableCell>
          <TableCell align="right">
            {/* No interval: wickets are a count distribution on this path, so the
                probabilities are shown and nothing is derived from them (P1-4). */}
            <Typography variant="body2" component="span">
              {ground.candidate.wickets.expected.toFixed(2)}
            </Typography>
            <Typography variant="caption" color="text.secondary" component="div">
              P(0) {ground.candidate.wickets.p0.toFixed(2)} · P(1){' '}
              {ground.candidate.wickets.p1.toFixed(2)} · P(2+){' '}
              {ground.candidate.wickets.p2_plus.toFixed(2)}
            </Typography>
          </TableCell>
          <TableCell align="right">
            <Quantiles quantiles={ground.eleven_total.total} />
            <Typography variant="caption" color="text.secondary" component="div">
              {ground.eleven_total.samples} draws
            </Typography>
          </TableCell>
          <TableCell align="right">{ground.eleven_total.spread_share.toFixed(3)}</TableCell>
        </TableRow>
      ))}
    </TableBody>
  </Table>
);

const AuctionProjectionPanel: React.FC<AuctionProjectionPanelProps> = ({
  listed,
  projection,
  projecting,
  error,
  onProject,
}) => {
  const [candidate, setCandidate] = React.useState('');
  const [toss, setToss] = React.useState<AuctionProjectionToss>('unknown');

  const tossFlag = toss === 'unknown' ? undefined : toss === 'candidate_eleven_bats_first';

  return (
    <Box sx={{ mt: 2 }} data-testid="auction-projection">
      <Typography variant="subtitle2">Project a candidate</Typography>
      <Typography variant="body2" color="text.secondary">
        What the performance model forecasts he would produce in the eleven named above, and what
        the simulator draws for that eleven with him in it. Every range is the stack’s as served.
      </Typography>

      <Stack
        direction="row"
        spacing={1}
        sx={{ mt: 1 }}
        alignItems="center"
        flexWrap="wrap"
        useFlexGap
      >
        <TextField
          select
          size="small"
          label="Candidate"
          value={candidate}
          sx={{ minWidth: 220 }}
          onChange={(event) => setCandidate(event.target.value)}
        >
          {listed.map((player) => (
            <MenuItem key={player.player_id} value={player.player_id}>
              {player.player_name}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          size="small"
          label="Toss"
          value={toss}
          sx={{ minWidth: 260 }}
          onChange={(event) => setToss(event.target.value as AuctionProjectionToss)}
        >
          {(Object.keys(TOSS_LABELS) as AuctionProjectionToss[]).map((value) => (
            <MenuItem key={value} value={value}>
              {TOSS_LABELS[value]}
            </MenuItem>
          ))}
        </TextField>
        <Button
          variant="contained"
          size="small"
          disabled={projecting || !candidate}
          data-testid="auction-project"
          onClick={() => onProject({ playerId: Number(candidate), team1BatsFirst: tossFlag })}
        >
          Project
        </Button>
      </Stack>

      {/* A refused projection shows no number at all: the refusal names the assumption
          that is missing or the state that is stale, and nothing stands in for it. */}
      <ErrorNotice error={error} title="No projection was made" />

      {projection && !error && (
        <>
          <Assumptions projection={projection} />
          <GroundRows projection={projection} />
          {projection.mixture && (
            <Box sx={{ mt: 1 }} data-testid="auction-mixture">
              <Typography variant="body2">
                Over the named grounds: {projection.mixture.total.median.toFixed(1)} (
                {projection.mixture.total.q10.toFixed(1)}–{projection.mixture.total.q90.toFixed(1)})
              </Typography>
              <Typography variant="caption" color="text.secondary" component="div">
                {projection.mixture.note}
              </Typography>
            </Box>
          )}
          <IntervalSources projection={projection} />
          <Typography variant="caption" component="div" sx={{ mt: 1 }}>
            <RatingsAsOf served={projection.served_ratings} />
          </Typography>
        </>
      )}
    </Box>
  );
};

export default AuctionProjectionPanel;
