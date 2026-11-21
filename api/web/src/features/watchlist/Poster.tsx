import type { MediaKind } from '../../lib/types';
import { Icon } from '../../components/Icon';

/** A small poster, or the movie/show icon when there is none. */
export function Poster({ url, kind, large = false }: { url?: string; kind: MediaKind; large?: boolean }) {
  return (
    <span className={`poster${large ? ' large' : ''}`} aria-hidden="true">
      {url ? <img src={url} alt="" loading="lazy" /> : <Icon name={kind === 'movie' ? 'film' : 'tv'} size={large ? 40 : 22} />}
    </span>
  );
}
