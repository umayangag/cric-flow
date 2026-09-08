import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { Mock } from 'vitest';
import { api } from './api';

/**
 * The auction client (P3-1).
 *
 * What is worth pinning here is the shape of each request — the path, the method and the
 * body — because the operator is typing facts during a live auction and a field dropped
 * on the way out is a sale the record never hears about.
 */
describe('the auction client', () => {
  let fetchMock: Mock;

  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock;
  });

  function requestFor(call: number): { url: string; init: RequestInit } {
    const [url, init] = fetchMock.mock.calls[call] as [string, RequestInit];
    return { url, init };
  }

  it('lists the auctions and reads one whole', async () => {
    await api.auctions();
    await api.auction('auction-1');

    expect(requestFor(0).url).toContain('/api/auctions');
    expect(requestFor(1).url).toContain('/api/auctions/auction-1');
  });

  it('creates an auction with its format, buying side and squad constraints', async () => {
    await api.createAuction({
      name: 'IPL 2027',
      format: 'T20',
      buyer_club_id: 11,
      squad_size: 25,
      min_bowlers: 5,
      require_keeper: true,
    });

    const { url, init } = requestFor(0);
    expect(url).toContain('/api/auctions');
    expect(init.method).toBe('POST');
    expect(JSON.parse(String(init.body))).toEqual({
      name: 'IPL 2027',
      format: 'T20',
      buyer_club_id: 11,
      squad_size: 25,
      min_bowlers: 5,
      require_keeper: true,
    });
  });

  it('lists players onto an auction by id', async () => {
    await api.addAuctionPlayers('auction-1', [7, 8]);

    const { url, init } = requestFor(0);
    expect(url).toContain('/api/auctions/auction-1/players');
    expect(JSON.parse(String(init.body))).toEqual({ player_ids: [7, 8] });
  });

  it('records a sale with its buyer and price, and an undo with neither', async () => {
    await api.recordAuctionOutcome('auction-1', {
      player_id: 7,
      state: 'sold',
      buyer_name: 'Rival',
      buyer_club_id: 12,
      price: 450,
    });
    await api.recordAuctionOutcome('auction-1', { player_id: 7, state: 'available' });

    expect(requestFor(0).url).toContain('/api/auctions/auction-1/outcomes');
    expect(JSON.parse(String(requestFor(0).init.body))).toEqual({
      player_id: 7,
      state: 'sold',
      buyer_name: 'Rival',
      buyer_club_id: 12,
      price: 450,
    });
    expect(JSON.parse(String(requestFor(1).init.body))).toEqual({
      player_id: 7,
      state: 'available',
    });
  });

  it('searches players across clubs, narrowing to a format when one is named', async () => {
    await api.searchPlayers({ q: 'kohli', format: 'T20', limit: 10 });
    await api.searchPlayers({ q: 'kohli' });

    expect(requestFor(0).url).toContain('/api/players/search?q=kohli&format=T20&limit=10');
    expect(requestFor(1).url).toContain('/api/players/search?q=kohli');
    expect(requestFor(1).url).not.toContain('format=');
  });
});
