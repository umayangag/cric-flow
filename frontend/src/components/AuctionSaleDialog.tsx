import React from 'react';
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import type { AuctionListedPlayer } from '../types';

/**
 * Record what the room paid, and who paid it (P3-1).
 *
 * Both are facts the operator observed: the buyer is typed as the auctioneer names it, and
 * the price is a whole number in the room's own unit — no currency is implied, because the
 * system has none. The switch says whether the buyer is this auction's own side, which is
 * the only thing that puts a purchase in the squad; every other franchise is a name.
 *
 * There is no suggested price here, and there will not be one on this surface: a price is
 * a fact the operator enters, and the value it is later compared against is P3-4's, with
 * its range and the *n* behind it.
 */

export type AuctionSaleDialogProps = {
  open: boolean;
  onClose: () => void;
  player: AuctionListedPlayer | null;
  /** The auction's own side, and its name, for the "this is my purchase" switch. */
  buyer: { club_id: number; name: string };
  onConfirm: (sale: { buyerName: string; buyerClubId?: number; price: number }) => void;
};

const AuctionSaleDialog: React.FC<AuctionSaleDialogProps> = ({
  open,
  onClose,
  player,
  buyer,
  onConfirm,
}) => {
  const [buyerName, setBuyerName] = React.useState('');
  const [price, setPrice] = React.useState('');
  const [toOwnSide, setToOwnSide] = React.useState(false);

  React.useEffect(() => {
    if (!open) return;
    setBuyerName('');
    setPrice('');
    setToOwnSide(false);
  }, [open]);

  const resolvedBuyerName = toOwnSide ? buyer.name : buyerName.trim();
  const parsedPrice = Number.parseInt(price, 10);
  const priceIsWhole = /^\d+$/.test(price.trim());

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>Sold: {player?.player_name}</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <FormControlLabel
            control={
              <Switch
                checked={toOwnSide}
                onChange={(event) => setToOwnSide(event.target.checked)}
                inputProps={{ 'aria-label': 'bought by your side' }}
              />
            }
            label={`Bought by ${buyer.name} (your squad)`}
          />
          {!toOwnSide && (
            <TextField
              size="small"
              autoFocus
              label="Buyer"
              value={buyerName}
              onChange={(event) => setBuyerName(event.target.value)}
              helperText="The franchise as the auctioneer named it."
            />
          )}
          <TextField
            size="small"
            label="Price"
            value={price}
            onChange={(event) => setPrice(event.target.value)}
            inputProps={{ inputMode: 'numeric' }}
            helperText="A whole number in the auction room's own unit; no currency is implied."
          />
          <Typography variant="caption" color="text.secondary">
            This is recorded exactly as entered. Nothing here is a valuation, and the record says
            nothing about whether the purchase was good.
          </Typography>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          disabled={resolvedBuyerName === '' || !priceIsWhole}
          onClick={() => {
            onConfirm({
              buyerName: resolvedBuyerName,
              buyerClubId: toOwnSide ? buyer.club_id : undefined,
              price: parsedPrice,
            });
            onClose();
          }}
        >
          Record the sale
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AuctionSaleDialog;
