export const LABEL_KEYS = {
  NAME:    'opni.io/name',
  VERSION: 'opni.io/agent-version'
};

export type Labels = { [key: string]: string };

export interface Reference {
  id: string;
}

export interface Timestamp {
  seconds: number;
  nanos: number;
}

export interface Duration {
  seconds: number;
  nanos: number;
}

export type State = 'success' | 'warning' | 'error';

export interface Status {
  state: State;
  message: string;
  longMessage?: string;
}
