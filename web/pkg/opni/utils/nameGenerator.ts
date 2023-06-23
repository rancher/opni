import {
  uniqueNamesGenerator, Config, adjectives, colors, animals
} from 'unique-names-generator';

const config: Config = { dictionaries: [adjectives, colors, animals] };

export function generateName() {
  return uniqueNamesGenerator(config);
}
